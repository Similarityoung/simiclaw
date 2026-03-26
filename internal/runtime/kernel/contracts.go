package kernel

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

// Executor runs already-claimed work outside the storage transaction boundary.
type Executor interface {
	Execute(ctx context.Context, claim runtimemodel.ClaimContext, sink EventSink) (runtimemodel.ExecutionResult, error)
}

// Processor owns the runtime claim -> execute -> finalize orchestration for one
// already-scheduled work item.
type Processor interface {
	Process(ctx context.Context, work runtimemodel.WorkItem) error
}

// Worker owns one bounded background responsibility inside the runtime.
type Worker interface {
	Role() WorkerRole
	Run(ctx context.Context) error
}

type WorkerRole struct {
	Name          string
	HeartbeatName string
	PollCadence   time.Duration
	FailurePolicy string
}

type ParameterSchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Minimum     *float64                   `json:"minimum,omitempty"`
	Maximum     *float64                   `json:"maximum,omitempty"`
	Properties  map[string]ParameterSchema `json:"properties,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Items       *ParameterSchema           `json:"items,omitempty"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  ParameterSchema `json:"parameters"`
}

type ToolContext struct {
	Workspace    string
	Scopes       []string
	Conversation model.Conversation
}

type ToolResult struct {
	Disabled bool              `json:"disabled"`
	Output   map[string]any    `json:"output,omitempty"`
	Error    *model.ErrorBlock `json:"error,omitempty"`
}

type ToolCatalog interface {
	ToolDefinitions() []ToolDefinition
	Invoke(ctx context.Context, toolCtx ToolContext, name string, args map[string]any) ToolResult
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ModelMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []model.ToolCall `json:"tool_calls,omitempty"`
}

type ModelRequest struct {
	Model    string           `json:"model"`
	Messages []ModelMessage   `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type ModelResult struct {
	Text              string           `json:"text"`
	ToolCalls         []model.ToolCall `json:"tool_calls,omitempty"`
	FinishReason      string           `json:"finish_reason"`
	RawFinishReason   string           `json:"raw_finish_reason"`
	Usage             Usage            `json:"usage"`
	Provider          string           `json:"provider"`
	Model             string           `json:"model"`
	ProviderRequestID string           `json:"provider_request_id"`
}

type ModelStreamSink interface {
	OnReasoningDelta(delta string)
	OnTextDelta(delta string)
}

type ModelInvoker interface {
	Chat(ctx context.Context, req ModelRequest) (ModelResult, error)
	StreamChat(ctx context.Context, req ModelRequest, sink ModelStreamSink) (ModelResult, error)
}

type ModelResolver interface {
	DefaultModel() string
	Resolve(model string) (ModelInvoker, string, error)
}

type CapabilityErrorKind string

const (
	CapabilityErrorFailed          CapabilityErrorKind = "error"
	CapabilityErrorTimeout         CapabilityErrorKind = "timeout"
	CapabilityErrorCanceled        CapabilityErrorKind = "canceled"
	CapabilityErrorInvalidResponse CapabilityErrorKind = "invalid_response"
)

type CapabilityError struct {
	Source     string
	Capability string
	Operation  string
	Kind       CapabilityErrorKind
	Err        error
}

func (e *CapabilityError) Error() string {
	if e == nil {
		return ""
	}
	scopeParts := make([]string, 0, 2)
	if source := strings.TrimSpace(e.Source); source != "" {
		scope := source
		if capability := strings.TrimSpace(e.Capability); capability != "" {
			scope += "." + capability
		}
		scopeParts = append(scopeParts, scope)
	} else if capability := strings.TrimSpace(e.Capability); capability != "" {
		scopeParts = append(scopeParts, capability)
	}
	if operation := strings.TrimSpace(e.Operation); operation != "" {
		scopeParts = append(scopeParts, operation)
	}
	prefix := "capability"
	if len(scopeParts) > 0 {
		prefix = strings.Join(scopeParts, " ")
	}

	suffix := "failed"
	switch e.Kind {
	case CapabilityErrorTimeout:
		suffix = "timed out"
	case CapabilityErrorCanceled:
		suffix = "canceled"
	case CapabilityErrorInvalidResponse:
		suffix = "returned invalid response"
	}
	if e.Err == nil {
		return prefix + " " + suffix
	}
	return prefix + " " + suffix + ": " + e.Err.Error()
}

func (e *CapabilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func WrapCapabilityError(source, capability, operation string, err error) error {
	if err == nil {
		return nil
	}
	var capErr *CapabilityError
	if errors.As(err, &capErr) {
		return capErr
	}
	return &CapabilityError{
		Source:     source,
		Capability: capability,
		Operation:  operation,
		Kind:       CapabilityErrorKindOf(err),
		Err:        err,
	}
}

func NewCapabilityError(source, capability, operation string, kind CapabilityErrorKind, err error) *CapabilityError {
	if kind == "" {
		kind = CapabilityErrorKindOf(err)
	}
	return &CapabilityError{
		Source:     source,
		Capability: capability,
		Operation:  operation,
		Kind:       kind,
		Err:        err,
	}
}

func CapabilityErrorKindOf(err error) CapabilityErrorKind {
	if err == nil {
		return ""
	}
	var capErr *CapabilityError
	if errors.As(err, &capErr) && capErr.Kind != "" {
		return capErr.Kind
	}
	switch {
	case errors.Is(err, context.Canceled):
		return CapabilityErrorCanceled
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return CapabilityErrorTimeout
	default:
		return CapabilityErrorFailed
	}
}

func ErrorBlockFromError(err error) *model.ErrorBlock {
	if err == nil {
		return nil
	}
	code := model.ErrorCodeInternal
	switch CapabilityErrorKindOf(err) {
	case CapabilityErrorTimeout, CapabilityErrorCanceled:
		code = model.ErrorCodeCanceled
	}
	var details map[string]any
	var capErr *CapabilityError
	if errors.As(err, &capErr) {
		details = map[string]any{
			"capability_error_kind": string(capErr.Kind),
		}
		if capErr.Source != "" {
			details["capability_source"] = capErr.Source
		}
		if capErr.Capability != "" {
			details["capability"] = capErr.Capability
		}
		if capErr.Operation != "" {
			details["capability_operation"] = capErr.Operation
		}
	}
	return &model.ErrorBlock{
		Code:    code,
		Message: err.Error(),
		Details: details,
	}
}
