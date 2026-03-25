package runner

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var idSeq atomic.Uint64

func cloneToolCalls(in []model.ToolCall) []model.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.ToolCall, 0, len(in))
	for _, call := range in {
		cloned := call
		if call.Args != nil {
			cloned.Args = cloneMap(call.Args)
		}
		out = append(out, cloned)
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func nextID(prefix string, now time.Time) string {
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}
