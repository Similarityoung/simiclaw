package lanes

import (
	"testing"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func TestResolvePrefersExplicitLaneKey(t *testing.T) {
	work := runtimemodel.WorkItem{
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
		LaneKey:    "lane:manual",
	}

	if got := Resolve(work); got != Key("lane:manual") {
		t.Fatalf("expected explicit lane key, got %q", got)
	}
}

func TestResolveUsesSessionKeyBeforeEventID(t *testing.T) {
	work := runtimemodel.WorkItem{
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
	}

	if got := Resolve(work); got != Key("session:local:dm:u1") {
		t.Fatalf("expected session lane key, got %q", got)
	}
}

func TestResolveFallsBackToEventID(t *testing.T) {
	work := runtimemodel.WorkItem{EventID: "evt_1"}
	if got := Resolve(work); got != Key("event:evt_1") {
		t.Fatalf("expected event lane key, got %q", got)
	}
}
