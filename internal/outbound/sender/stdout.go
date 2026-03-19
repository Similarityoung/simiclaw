package sender

import (
	"context"
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Sender interface {
	Send(ctx context.Context, msg model.OutboxMessage) error
}

type Stdout struct{}

func (s Stdout) Send(_ context.Context, msg model.OutboxMessage) error {
	if strings.Contains(msg.Body, "[fail_outbound]") {
		return fmt.Errorf("simulated outbound failure")
	}
	return nil
}
