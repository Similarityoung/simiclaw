package kernel

import (
	"errors"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestCapabilityErrorKindOfDoesNotGuessFromGenericErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "timeout word only", err: errors.New("prompt timeout budget exceeded")},
		{name: "canceled word only", err: errors.New("user note says canceled by operator")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CapabilityErrorKindOf(tc.err); got != CapabilityErrorFailed {
				t.Fatalf("CapabilityErrorKindOf() = %q want %q", got, CapabilityErrorFailed)
			}
		})
	}
}

func TestErrorBlockFromErrorKeepsGenericErrorsInternal(t *testing.T) {
	block := ErrorBlockFromError(errors.New("timeout while loading cached prompt"))
	if block == nil {
		t.Fatal("expected error block")
	}
	if block.Code != model.ErrorCodeInternal {
		t.Fatalf("code = %q want %q", block.Code, model.ErrorCodeInternal)
	}
}
