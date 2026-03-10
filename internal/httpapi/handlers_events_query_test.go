package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
)

func TestListEventsCursorUsesCreatedAtAnchor(t *testing.T) {
	app, srv := newHTTPAPITestServer(t)
	conversation := "events_cursor_test"

	var eventIDs []string
	for i := 1; i <= 3; i++ {
		eventID := ingestTestEvent(t, srv.URL, buildCLIRequest(conversation, i, "message", fmt.Sprintf("hello %d", i)))
		waitEventTerminal(t, app, eventID)
		eventIDs = append(eventIDs, eventID)
	}

	if _, err := app.DB.Writer().ExecContext(
		context.Background(),
		`UPDATE events SET updated_at = ? WHERE event_id = ?`,
		time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339Nano),
		eventIDs[1],
	); err != nil {
		t.Fatalf("update middle event updated_at: %v", err)
	}

	page1 := fetchEventPage(t, srv.URL, "", 2)
	if len(page1.Items) != 2 {
		t.Fatalf("expected two events on first page, got %+v", page1.Items)
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected next_cursor on first page")
	}

	page2 := fetchEventPage(t, srv.URL, page1.NextCursor, 2)
	if len(page2.Items) != 1 {
		t.Fatalf("expected one event on second page, got %+v", page2.Items)
	}
	seen := map[string]bool{}
	for _, item := range page1.Items {
		seen[item.EventID] = true
	}
	if seen[page2.Items[0].EventID] {
		t.Fatalf("expected created_at cursor to avoid duplicates, got first=%+v second=%+v", page1.Items, page2.Items)
	}
}

func fetchEventPage(t *testing.T, baseURL, cursor string, limit int) struct {
	Items      []api.EventRecord `json:"items"`
	NextCursor string            `json:"next_cursor"`
} {
	t.Helper()
	url := fmt.Sprintf("%s/v1/events?limit=%d", baseURL, limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("events status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		Items      []api.EventRecord `json:"items"`
		NextCursor string            `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	return out
}
