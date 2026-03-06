package adkruntime

import (
	"context"
	"iter"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type LocalEchoModel struct{}

func (LocalEchoModel) Name() string {
	return "simiclaw-local-echo"
}

func (LocalEchoModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		text := strings.TrimSpace(lastUserText(req))
		if text == "" {
			text = "(no reply)"
		}
		resp := &model.LLMResponse{
			Content: genai.NewContentFromText("已收到: "+text, genai.RoleModel),
		}
		yield(resp, nil)
	}
}

func lastUserText(req *model.LLMRequest) string {
	if req == nil || len(req.Contents) == 0 {
		return ""
	}
	for i := len(req.Contents) - 1; i >= 0; i-- {
		c := req.Contents[i]
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p != nil && strings.TrimSpace(p.Text) != "" {
				return p.Text
			}
		}
	}
	return ""
}
