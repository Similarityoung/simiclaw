package context

import (
	"context"
	"strings"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
)

type Assembler struct {
	reader       Reader
	historyLimit int
	ragLimit     int
}

func NewAssembler(reader Reader) Assembler {
	return Assembler{
		reader:       reader,
		historyLimit: 20,
		ragLimit:     5,
	}
}

func (a Assembler) Load(ctx context.Context, sessionID, query string) (Bundle, error) {
	history, err := a.reader.LoadPromptHistory(ctx, sessionID, a.historyLimit)
	if err != nil {
		return Bundle{}, err
	}
	ragHits, _ := a.reader.SearchRAGHits(ctx, sessionID, strings.TrimSpace(query), a.ragLimit)
	return Bundle{
		History: history,
		RAGHits: ragHits,
		Manifest: &runnermodel.ContextManifest{
			HistoryRange: runnermodel.HistoryRange{Mode: "tail", TailLimit: a.historyLimit},
		},
	}, nil
}
