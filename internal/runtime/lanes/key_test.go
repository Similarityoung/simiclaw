package lanes

import (
	"testing"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func TestResolvePrefersExplicitLaneKey(t *testing.T) {
	work := runtimemodel.WorkItem{
		Kind:       runtimemodel.WorkKindEvent,
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
		LaneKey:    "lane:manual",
	}

	if got := Resolve(work); got != Key("lane:manual") {
		t.Fatalf("expected explicit lane key, got %q", got)
	}
}

func TestResolveUsesSessionKeyBeforeEventIdentity(t *testing.T) {
	work := runtimemodel.WorkItem{
		Kind:       runtimemodel.WorkKindEvent,
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
	}

	if got := Resolve(work); got != Key("session:local:dm:u1") {
		t.Fatalf("expected session lane key, got %q", got)
	}
}

func TestResolveFallsBackByWorkKind(t *testing.T) {
	cases := []struct {
		name string
		work runtimemodel.WorkItem
		want Key
	}{
		{
			name: "event",
			work: runtimemodel.WorkItem{Kind: runtimemodel.WorkKindEvent, EventID: "evt_1"},
			want: Key("event:evt_1"),
		},
		{
			name: "outbox",
			work: runtimemodel.WorkItem{Kind: runtimemodel.WorkKindOutbox, OutboxID: "out_1"},
			want: Key("outbox:out_1"),
		},
		{
			name: "scheduled job",
			work: runtimemodel.WorkItem{Kind: runtimemodel.WorkKindScheduledJob, JobID: "job_1"},
			want: Key("job:job_1"),
		},
		{
			name: "recovery",
			work: runtimemodel.WorkItem{Kind: runtimemodel.WorkKindRecovery, EventID: "evt_recover"},
			want: Key("recovery:evt_recover"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.work); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
