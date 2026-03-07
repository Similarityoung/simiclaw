package gateway

const (
	retryAfterSeconds       = 1
	statusClientClosedReq   = 499
	ingestStatusAccepted    = "accepted"
	ingestStatusDuplicate   = "duplicate_acked"
	ingestStatusURLTemplate = "/v1/events/%s"
	msgIdempotencyConflict  = "idempotency payload hash mismatch"
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
