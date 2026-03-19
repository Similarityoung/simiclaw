package context

import (
	"time"

	"github.com/similarityyoung/simiclaw/internal/memory"
)

// MemoryWriter keeps the runner's long-term memory writes behind one explicit
// boundary. Prompt-time memory injection remains owned by internal/prompt.
type MemoryWriter struct {
	writer *memory.Writer
}

func NewMemoryWriter(workspace string) MemoryWriter {
	return MemoryWriter{writer: memory.NewWriter(workspace)}
}

func (w MemoryWriter) WriteDaily(source, note string, now time.Time, channelType string) error {
	if w.writer == nil {
		return nil
	}
	_, err := w.writer.WriteDaily(source, note, now, memory.VisibilityForChannel(channelType))
	return err
}

func (w MemoryWriter) WriteCurated(note string, now time.Time, channelType string) error {
	if w.writer == nil {
		return nil
	}
	_, err := w.writer.WriteCurated(note, now, memory.VisibilityForChannel(channelType))
	return err
}
