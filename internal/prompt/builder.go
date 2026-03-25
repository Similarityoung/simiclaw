package prompt

import (
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var workspaceContextFiles = []string{"SOUL.md", "IDENTITY.md", "USER.md", "AGENTS.md", "TOOLS.md"}

const (
	bootstrapContextFile = "BOOTSTRAP.md"
	heartbeatContextFile = "HEARTBEAT.md"
)

type Builder struct {
	workspace string
	loader    promptLoader
	finger    promptFingerprinter
	renderer  promptRenderer

	mu           sync.RWMutex
	cachedStatic map[string]staticCacheEntry
	staticBuilds int
}

type staticCacheEntry struct {
	content      string
	fingerprints map[string]string
}

type staticVariant struct {
	memoryMode       string
	includeHeartbeat bool
}

func (v staticVariant) key() string {
	if v.includeHeartbeat {
		return v.memoryMode + "|heartbeat"
	}
	return v.memoryMode + "|normal"
}

type RunContext struct {
	Now          time.Time
	Conversation model.Conversation
	SessionKey   string
	SessionID    string
	PayloadType  string
}

type BuildInput struct {
	Context RunContext
}

type SkillSummary struct {
	Name        string
	Description string
	Path        string
}

type textEntry struct {
	DisplayPath  string
	ResolvedPath string
	Content      string
}

type staticContextBundle struct {
	workspacePath    string
	memoryBlocks     []textEntry
	workspaceContext []textEntry
	skills           []SkillSummary
	heartbeat        textEntry
	hasHeartbeat     bool
	includeHeartbeat bool
}

func NewBuilder(workspace string) *Builder {
	builder := &Builder{
		workspace:    workspace,
		cachedStatic: map[string]staticCacheEntry{},
	}
	builder.loader = promptLoader{workspace: workspace}
	builder.finger = promptFingerprinter{workspace: workspace}
	builder.renderer = promptRenderer{}
	return builder
}

func (b *Builder) Build(input BuildInput) string {
	now := input.Context.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	variant := buildStaticVariant(input.Context)
	parts := []string{
		b.renderStaticPrefix(variant),
		b.renderer.renderCurrentRunContext(input.Context, now),
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func buildStaticVariant(ctx RunContext) staticVariant {
	return staticVariant{
		memoryMode:       buildMemoryMode(ctx.Conversation.ChannelType),
		includeHeartbeat: strings.EqualFold(strings.TrimSpace(ctx.PayloadType), "cron_fire"),
	}
}

func (b *Builder) loadStaticBundle(variant staticVariant) staticContextBundle {
	return b.loader.loadStaticBundle(variant)
}

func (b *Builder) renderStaticPrefix(variant staticVariant) string {
	key := variant.key()
	snapshot := b.finger.snapshotStaticState(variant)

	b.mu.RLock()
	if entry, ok := b.cachedStatic[key]; ok && equalStringMap(entry.fingerprints, snapshot) {
		cached := entry.content
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	content, snapshot, cacheable := stableStaticBuild(
		func() string { return b.renderer.renderStatic(b.loadStaticBundle(variant)) },
		func() map[string]string { return b.finger.snapshotStaticState(variant) },
	)

	b.mu.Lock()
	defer b.mu.Unlock()
	latest := b.finger.snapshotStaticState(variant)
	if entry, ok := b.cachedStatic[key]; ok && equalStringMap(entry.fingerprints, latest) {
		return entry.content
	}
	if cacheable && equalStringMap(snapshot, latest) {
		b.cachedStatic[key] = staticCacheEntry{content: content, fingerprints: snapshot}
		b.staticBuilds++
	}
	return content
}
