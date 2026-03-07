package outbound

import (
	"context"
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Sender interface {
	Send(ctx context.Context, msg model.OutboxMessage) error
}

type StdoutSender struct{}

func (s StdoutSender) Send(_ context.Context, msg model.OutboxMessage) error {
	if strings.Contains(msg.Body, "[fail_outbound]") {
		return fmt.Errorf("simulated outbound failure")
	}
	return nil
}
