package httpapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/api"
)

func TestQueryAndHealthEndpoints(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)
	req := buildCLIRequest("query_endpoints", 1, "message", "hello query")
	eventID := ingestTestEvent(t, srv.URL, req)
	waitEventTerminal(t, app, eventID)

	event := fetchJSON[api.EventRecord](t, srv.URL+"/v1/events/"+eventID)
	if event.EventID != eventID {
		t.Fatalf("unexpected event payload: %+v", event)
	}

	lookup := fetchJSON[map[string]any](t, fmt.Sprintf("%s/v1/events:lookup?idempotency_key=%s", srv.URL, req.IdempotencyKey))
	if lookup["event_id"] != eventID {
		t.Fatalf("unexpected lookup payload: %+v", lookup)
	}

	runPage := fetchJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, srv.URL+"/v1/runs?limit=10")
	if len(runPage.Items) == 0 {
		t.Fatalf("expected runs list")
	}
	runID, _ := runPage.Items[0]["run_id"].(string)
	runSummary := fetchJSON[map[string]any](t, srv.URL+"/v1/runs/"+runID)
	if runSummary["run_id"] != runID {
		t.Fatalf("unexpected run summary: %+v", runSummary)
	}
	runTraceSummary := fetchJSON[map[string]any](t, srv.URL+"/v1/runs/"+runID+"/trace?view=summary")
	if runTraceSummary["run_id"] != runID {
		t.Fatalf("unexpected run trace summary: %+v", runTraceSummary)
	}
	runTraceFull := fetchJSON[map[string]any](t, srv.URL+"/v1/runs/"+runID+"/trace")
	if runTraceFull["run_id"] != runID {
		t.Fatalf("unexpected full run trace: %+v", runTraceFull)
	}

	sessionPage := fetchJSON[struct {
		Items []api.SessionRecord `json:"items"`
	}](t, srv.URL+"/v1/sessions?conversation_id=query_endpoints")
	if len(sessionPage.Items) != 1 {
		t.Fatalf("expected one session, got %+v", sessionPage.Items)
	}
	session := fetchJSON[api.SessionRecord](t, srv.URL+"/v1/sessions/"+sessionPage.Items[0].SessionKey)
	if session.SessionKey != sessionPage.Items[0].SessionKey {
		t.Fatalf("unexpected session payload: %+v", session)
	}

	health := fetchJSON[map[string]any](t, srv.URL+"/healthz")
	if health["status"] != "ok" {
		t.Fatalf("unexpected health payload: %+v", health)
	}
	ready := fetchJSON[map[string]any](t, srv.URL+"/readyz")
	if ready["status"] != "ready" {
		t.Fatalf("unexpected ready payload: %+v", ready)
	}
}

func fetchJSON[T any](t *testing.T, url string) T {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out
}
