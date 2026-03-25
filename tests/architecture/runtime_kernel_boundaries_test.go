package architecture

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGatewayProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/gateway")
}

func TestGatewayProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/gateway")
}

func TestHTTPProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/http")
}

func TestHTTPProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/http")
}

func TestRuntimeKernelProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/runtime/kernel")
}

func TestRuntimeKernelProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/runtime/kernel")
}

func TestRuntimePlaneProductionCodeDoesNotImportSurfaceAdapters(t *testing.T) {
	for _, dir := range []string{"internal/gateway", "internal/runtime", "internal/outbound"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/http",
			"github.com/similarityyoung/simiclaw/internal/channels",
			"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal",
		)
	}
}

func TestRuntimeCoreProductionCodeDoesNotImportContextStatePlane(t *testing.T) {
	for _, dir := range []string{"internal/gateway", "internal/runtime", "internal/outbound"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/query",
			"github.com/similarityyoung/simiclaw/internal/prompt",
			"github.com/similarityyoung/simiclaw/internal/memory",
			"github.com/similarityyoung/simiclaw/internal/workspace",
			"github.com/similarityyoung/simiclaw/internal/workspacefile",
		)
	}
}

func TestRuntimeCoreProductionCodeDoesNotImportCapabilityPlane(t *testing.T) {
	for _, dir := range []string{"internal/gateway", "internal/runtime", "internal/outbound"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/provider",
			"github.com/similarityyoung/simiclaw/internal/tools",
		)
	}
}

func TestStoreRootProductionCodeDoesNotImportRuntimeModel(t *testing.T) {
	root := repoRoot(t)
	files := goFilesUnder(t, root, "internal/store")
	var violations []string
	for _, rel := range files {
		if strings.HasPrefix(rel, "internal/store/tx/") {
			continue
		}
		if fileImportsPath(t, filepath.Join(root, rel), runtimeModelPath) {
			violations = append(violations, rel)
		}
	}
	if len(violations) == 0 {
		return
	}
	slices.Sort(violations)
	t.Fatalf("production code under internal/store (excluding tx) must not import %s:\n%s", runtimeModelPath, strings.Join(violations, "\n"))
}

func TestStoreTxProductionCodeDoesNotImportRuntimeRoot(t *testing.T) {
	assertNoPackageImport(t, runtimeImportPath, "internal/store/tx")
}
