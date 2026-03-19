package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacySupportingPackagesDoNotExist(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/streaming",
		"internal/systemprompt",
		"internal/contextfile",
		"internal/ui",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Fatalf("legacy supporting package path still exists: %s", rel)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}
