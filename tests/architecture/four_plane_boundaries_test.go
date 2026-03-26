package architecture

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type ownerPlane string

const (
	planeSurface      ownerPlane = "Surface"
	planeRuntime      ownerPlane = "Runtime"
	planeContextState ownerPlane = "Context/State"
	planeCapability   ownerPlane = "Capability Plane"
)

const (
	roleTransport = "transport"
	roleExecution = "execution"
	roleObserve   = "observe"
	roleFallback  = "fallback"
)

type moduleOwner struct {
	path  string
	plane ownerPlane
	roles []string
}

var fourPlaneModuleOwners = []moduleOwner{
	{path: "cmd/simiclaw/internal/chat", plane: planeSurface, roles: []string{roleTransport, roleFallback}},
	{path: "cmd/simiclaw/internal/client", plane: planeSurface, roles: []string{roleFallback}},
	{path: "cmd/simiclaw/internal/inspect", plane: planeSurface, roles: []string{roleTransport}},
	{path: "cmd/simiclaw/internal/messages", plane: planeSurface, roles: []string{roleTransport}},
	{path: "cmd/simiclaw/internal/root", plane: planeSurface, roles: []string{roleTransport}},
	{path: "internal/channels", plane: planeSurface, roles: []string{roleTransport}},
	{path: "internal/http", plane: planeSurface, roles: []string{roleTransport}},
	{path: "internal/http/stream", plane: planeSurface, roles: []string{roleTransport, roleObserve}},
	{path: "internal/gateway", plane: planeRuntime, roles: []string{roleExecution}},
	{path: "internal/outbound", plane: planeRuntime, roles: []string{roleExecution}},
	{path: "internal/runner", plane: planeRuntime, roles: []string{roleExecution}},
	{path: "internal/runtime", plane: planeRuntime, roles: []string{roleExecution, roleObserve}},
	{path: "internal/store", plane: planeContextState},
	{path: "internal/query", plane: planeContextState, roles: []string{roleFallback}},
	{path: "internal/prompt", plane: planeContextState},
	{path: "internal/memory", plane: planeContextState},
	{path: "internal/workspace", plane: planeContextState},
	{path: "internal/workspacefile", plane: planeContextState},
	{path: "internal/provider", plane: planeCapability},
	{path: "internal/tools", plane: planeCapability},
}

func TestFourPlaneOwnerMapCoversCurrentMajorModules(t *testing.T) {
	root := repoRoot(t)
	for _, owner := range fourPlaneModuleOwners {
		files := goFilesUnder(t, root, owner.path)
		if len(files) == 0 {
			t.Fatalf("owner map entry %s (%s) has no production Go files", owner.path, owner.plane)
		}
	}
}

func TestSurfaceAdaptersDoNotImportContextAssets(t *testing.T) {
	for _, dir := range []string{"internal/http", "internal/channels"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/store",
			"github.com/similarityyoung/simiclaw/internal/prompt",
			"github.com/similarityyoung/simiclaw/internal/memory",
			"github.com/similarityyoung/simiclaw/internal/workspace",
			"github.com/similarityyoung/simiclaw/internal/workspacefile",
		)
	}
}

func TestSurfaceAdaptersDoNotImportCapabilityPlane(t *testing.T) {
	for _, dir := range []string{"internal/http", "internal/channels"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/provider",
			"github.com/similarityyoung/simiclaw/internal/tools",
		)
	}
}

func TestCapabilityPlaneDoesNotImportSurfaceAdapters(t *testing.T) {
	for _, dir := range []string{"internal/provider", "internal/tools"} {
		assertNoPackageImportPrefix(t, dir,
			"github.com/similarityyoung/simiclaw/internal/http",
			"github.com/similarityyoung/simiclaw/internal/channels",
			"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal",
		)
	}
}

func assertNoPackageImportPrefix(t *testing.T, dir string, prefixes ...string) {
	t.Helper()
	root := repoRoot(t)
	files := goFilesUnder(t, root, dir)
	if len(files) == 0 {
		t.Fatalf("expected production files under %s", dir)
	}

	var violations []string
	for _, rel := range files {
		for _, imported := range fileImportsMatchingPrefixes(t, filepath.Join(root, rel), prefixes...) {
			violations = append(violations, rel+" -> "+imported)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("%s production code must not import disallowed package prefixes:\n%s", dir, strings.Join(violations, "\n"))
	}
}

func fileImportsMatchingPrefixes(t *testing.T, absPath string, prefixes ...string) []string {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", absPath, err)
	}

	var matches []string
	for _, spec := range parsed.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		for _, prefix := range prefixes {
			if strings.HasPrefix(importPath, prefix) {
				matches = append(matches, importPath)
				break
			}
		}
	}
	return matches
}
