//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestCLINormalizeMessageFeedsGatewayDirectly(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().UTC()
	normalized := cli.NormalizeMessage("cli-direct-gateway", "u9", 1, "hello direct", now)

	accepted, apiErr := app.Gateway.Accept(context.Background(), normalized)
	if apiErr != nil {
		t.Fatalf("gateway accept: %+v", apiErr)
	}
	event := pollEvent(t, app, accepted.Response.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed event from cli normalized ingress, got %+v", event)
	}
	if event.SessionKey != accepted.Response.SessionKey {
		t.Fatalf("expected session key to align with gateway response, event=%+v accepted=%+v", event, accepted)
	}
}
