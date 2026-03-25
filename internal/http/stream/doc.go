// Package stream owns the Surface-plane HTTP streaming adapter.
//
// It is responsible for request/response transport details such as SSE headers,
// keepalive comments, and runtime-event framing. It must not become a new owner
// of runtime execution, durable state transitions, or client-side retry/poll
// fallback.
package stream
