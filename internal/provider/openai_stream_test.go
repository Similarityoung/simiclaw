package provider

import (
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestToolCallAccumulatorBindsLateToolCallID(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"par"}}]}}]}`))
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_1","index":0,"type":"function","function":{"name":"search","arguments":"is\"}"}}],"content":""},"finish_reason":"tool_calls"}]}`))

	toolCalls, err := acc.BuildToolCalls(0)
	if err != nil {
		t.Fatalf("BuildToolCalls returned error: %v", err)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id=call_1, got %+v", toolCalls[0])
	}
	if toolCalls[0].Name != "search" {
		t.Fatalf("expected tool name search, got %+v", toolCalls[0])
	}
	if got := toolCalls[0].Args["q"]; got != "paris" {
		t.Fatalf("expected parsed args q=paris, got %+v", toolCalls[0].Args)
	}
}

func TestToolCallAccumulatorDoesNotCrossChoices(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_2","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"beijing\"}"}}]}},{"index":1,"delta":{"tool_calls":[{"id":"call_b","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"shanghai\"}"}}]}}]}`))

	choice0, err := acc.BuildToolCalls(0)
	if err != nil {
		t.Fatalf("choice0 BuildToolCalls error: %v", err)
	}
	choice1, err := acc.BuildToolCalls(1)
	if err != nil {
		t.Fatalf("choice1 BuildToolCalls error: %v", err)
	}
	if len(choice0) != 1 || len(choice1) != 1 {
		t.Fatalf("unexpected tool call counts: choice0=%d choice1=%d", len(choice0), len(choice1))
	}
	if choice0[0].Args["city"] != "beijing" || choice1[0].Args["city"] != "shanghai" {
		t.Fatalf("tool args crossed choices: choice0=%+v choice1=%+v", choice0[0].Args, choice1[0].Args)
	}
}

func TestToolCallAccumulatorRejectsInvalidParallelArguments(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_3","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"beijing\"}"}},{"id":"call_b","index":1,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"shanghai\""}}]}}]}`))

	if _, err := acc.BuildToolCalls(0); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func mustChunk(t *testing.T, raw string) openai.ChatCompletionChunk {
	t.Helper()
	var chunk openai.ChatCompletionChunk
	if err := chunk.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	return chunk
}
