package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestHTTPAPIErrorPaths(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)

	postInvalidJSON(t, srv.URL+"/v1/events:ingest", http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/events/missing", http.StatusNotFound)
	getStatus(t, srv.URL+"/v1/events?cursor=bad", http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/events:lookup", http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/runs?cursor=bad", http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/sessions?cursor=bad", http.StatusBadRequest)
	getStatus(t, srv.URL+"/v1/sessions/missing", http.StatusNotFound)

	req := buildCLIRequest("error_paths", 1, "message", "hello")
	eventID := ingestTestEvent(t, srv.URL, req)
	waitEventTerminal(t, app, eventID)
	runPage := fetchJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, srv.URL+"/v1/runs?limit=10")
	if len(runPage.Items) == 0 {
		t.Fatalf("expected at least one run")
	}
	runID, _ := runPage.Items[0]["run_id"].(string)
	getStatus(t, srv.URL+"/v1/runs/"+runID+"/trace?redact=maybe", http.StatusBadRequest)
}

func postInvalidJSON(t *testing.T, url string, wantStatus int) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
}

func getStatus(t *testing.T, url string, wantStatus int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
	if resp.StatusCode >= 400 {
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode error body %s: %v", url, err)
		}
	}
}
