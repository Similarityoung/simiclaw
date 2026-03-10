package model

import pkgmodel "github.com/similarityyoung/simiclaw/pkg/model"

type HistoryMessage struct {
	Role       string
	Content    string
	ToolCalls  []pkgmodel.ToolCall
	ToolCallID string
	ToolName   string
	Meta       map[string]any
}

type RAGHit struct {
	Path    string  `json:"path"`
	Scope   string  `json:"scope"`
	Lines   []int   `json:"lines"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type HistoryRange struct {
	Mode      string `json:"mode"`
	TailLimit int    `json:"tail_limit,omitempty"`
}

type ContextManifest struct {
	HistoryRange HistoryRange `json:"history_range"`
}
