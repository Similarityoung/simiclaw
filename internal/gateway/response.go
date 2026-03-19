package gateway

const (
	statusClientClosedReq  = 499
	ingestStatusAccepted   = "accepted"
	ingestStatusDuplicate  = "duplicate_acked"
	msgIdempotencyConflict = "idempotency payload hash mismatch"
)

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]any
	RetryAfter int
}

func (e *APIError) Error() string {
	return e.Message
}
