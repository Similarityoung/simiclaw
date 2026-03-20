package logcapture

import (
	"os"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/logging"
	"go.uber.org/zap"
)

func TestCaptureStdoutRestoresGlobalLoggerAfterPanic(t *testing.T) {
	if err := logging.Init("info"); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	oldStdout := os.Stdout
	oldLogger := zap.L()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		_ = CaptureStdout(t, func() {
			if err := logging.Init("info"); err != nil {
				t.Fatalf("Init error: %v", err)
			}
			panic("boom")
		})
	}()

	if os.Stdout != oldStdout {
		t.Fatal("stdout was not restored")
	}
	if zap.L() != oldLogger {
		t.Fatal("global logger was not restored")
	}
}
