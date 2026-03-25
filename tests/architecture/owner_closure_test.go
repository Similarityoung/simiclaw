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

func TestFourPlaneOwnerMapDoesNotAssignMultiplePlanesToSameModule(t *testing.T) {
	seen := make(map[string]ownerPlane, len(fourPlaneModuleOwners))
	for _, owner := range fourPlaneModuleOwners {
		previous, ok := seen[owner.path]
		if ok && previous != owner.plane {
			t.Fatalf("module %s assigned to multiple planes: %s, %s", owner.path, previous, owner.plane)
		}
		seen[owner.path] = owner.plane
	}
}

func TestFourPlaneOwnerMapDoesNotCollapseTransportExecutionObserveAndFallback(t *testing.T) {
	for _, owner := range fourPlaneModuleOwners {
		if hasRole(owner.roles, roleTransport) &&
			hasRole(owner.roles, roleExecution) &&
			hasRole(owner.roles, roleObserve) &&
			hasRole(owner.roles, roleFallback) {
			t.Fatalf("module %s collapses transport/execution/observe/fallback into one owner", owner.path)
		}
	}
}

func hasRole(roles []string, want string) bool {
	for _, role := range roles {
		if role == want {
			return true
		}
	}
	return false
}
