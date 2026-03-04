package logging

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/multierr"
	"go.uber.org/zap"
)

type staticSyncErrorSink struct {
	syncErr error
}

type nonComparableError struct {
	items []int
	msg   string
}

func (e nonComparableError) Error() string {
	return e.msg
}

func (s staticSyncErrorSink) Write(p []byte) (int, error) {
	return len(p), nil
}

func (s staticSyncErrorSink) Sync() error {
	return s.syncErr
}

func TestSyncReturnsErrorWhenNonIgnorable(t *testing.T) {
	logger, err := newLogger("info", staticSyncErrorSink{syncErr: errors.New("disk full")})
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	syncErr := Sync()
	if syncErr == nil {
		t.Fatal("expected sync error")
	}
	if !strings.Contains(syncErr.Error(), "disk full") {
		t.Fatalf("unexpected sync error: %v", syncErr)
	}
}

func TestSyncIgnoresKnownStdStreamErrors(t *testing.T) {
	logger, err := newLogger("info", staticSyncErrorSink{syncErr: errors.New("bad file descriptor")})
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	if syncErr := Sync(); syncErr != nil {
		t.Fatalf("expected nil sync error, got %v", syncErr)
	}
}

func TestIsIgnorableSyncErrorHandlesMultierrAllIgnorable(t *testing.T) {
	err := multierr.Combine(errors.New("bad file descriptor"), errors.New("invalid argument"))
	if !isIgnorableSyncError(err) {
		t.Fatalf("expected ignorable multierr: %v", err)
	}
}

func TestIsIgnorableSyncErrorHandlesMultierrMixed(t *testing.T) {
	err := multierr.Combine(errors.New("bad file descriptor"), errors.New("disk full"))
	if isIgnorableSyncError(err) {
		t.Fatalf("expected non-ignorable multierr: %v", err)
	}
}

func TestIsIgnorableSyncErrorHandlesNonComparableError(t *testing.T) {
	err := nonComparableError{
		items: []int{1, 2, 3},
		msg:   "bad file descriptor",
	}
	if !isIgnorableSyncError(err) {
		t.Fatalf("expected ignorable non-comparable error: %v", err)
	}
}
