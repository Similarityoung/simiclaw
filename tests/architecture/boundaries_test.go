package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestOnlyGatewayServiceCallsPersistEventOutsideTests(t *testing.T) {
	root := repoRoot(t)
	files := goFilesUnder(t, root, "cmd", "internal")
	allowed := map[string]struct{}{
		"internal/gateway/service.go": {},
	}
	var violations []string

	for _, rel := range files {
		if _, ok := allowed[rel]; ok {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || selector.Sel.Name != "PersistEvent" {
				return true
			}
			pos := fset.Position(selector.Sel.Pos())
			violations = append(violations, rel+":"+itoa(pos.Line))
			return true
		})
	}

	if len(violations) == 0 {
		return
	}
	slices.Sort(violations)
	t.Fatalf("found direct PersistEvent calls outside allowed gateway entrypoints:\n%s", strings.Join(violations, "\n"))
}

func TestStoreProductionCodeDoesNotImportIngest(t *testing.T) {
	assertNoPackageImport(t, ingestImportPath, "internal/store")
}

func TestChannelsProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/channels")
}

func TestWorkspaceProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/workspace")
}

func TestQueryProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/query")
}

func TestRunnerProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/runner")
}

func TestRuntimeProductionCodeDoesNotImportStore(t *testing.T) {
	assertNoPackageImport(t, storeImportPath, "internal/runtime")
}

func TestRunnerProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/runner")
}

func TestEventLoopProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReferenceInFiles(t, "internal/runtime/eventloop.go")
}

func TestRuntimeProductionCodeDoesNotReferenceStoreDB(t *testing.T) {
	assertNoStoreDBReference(t, "internal/runtime")
}

func TestQueryExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/query")
}

func TestRunnerExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/runner")
}

func TestRuntimeExportedAPIDoesNotExposeStoreTypes(t *testing.T) {
	assertNoExportedImportSelectors(t, storeImportPath, "internal/runtime")
}

func TestOnlyBootstrapImportsStoreQueriesOutsideStore(t *testing.T) {
	assertOnlyAllowedProductionFilesImport(t, storeQueriesImportPath, map[string]struct{}{
		"internal/bootstrap/app.go": {},
	})
}

func TestOnlyBootstrapImportsStoreTxOutsideStore(t *testing.T) {
	assertOnlyAllowedProductionFilesImport(t, storeTxImportPath, map[string]struct{}{
		"internal/bootstrap/app.go": {},
	})
}

func TestStoreProductionCodeDoesNotImportAPI(t *testing.T) {
	assertNoPackageImport(t, apiImportPath, "internal/store")
}

func TestQueryModelProductionCodeDoesNotImportAPI(t *testing.T) {
	assertNoPackageImport(t, apiImportPath, "internal/query/model")
}

func TestChannelsProductionCodeDoesNotImportAPI(t *testing.T) {
	assertNoPackageImport(t, apiImportPath, "internal/channels")
}

func TestChatProductionCodeDoesNotImportNetHTTP(t *testing.T) {
	assertNoPackageImport(t, "net/http", "cmd/simiclaw/internal/chat")
}

func TestChatProductionCodeDoesNotImportNetURL(t *testing.T) {
	assertNoPackageImport(t, "net/url", "cmd/simiclaw/internal/chat")
}

func TestCommandPackagesDoNotImportStdlibFlag(t *testing.T) {
	root := repoRoot(t)
	files := goFilesUnder(t, root, "cmd/simiclaw/internal")
	var violations []string
	for _, rel := range files {
		if !strings.HasSuffix(rel, "/command.go") {
			continue
		}
		if fileImportsPath(t, filepath.Join(root, rel), "flag") {
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("command packages must not import stdlib flag:\n%s", strings.Join(violations, "\n"))
	}
}

const (
	storeImportPath        = "github.com/similarityyoung/simiclaw/internal/store"
	ingestImportPath       = "github.com/similarityyoung/simiclaw/internal/ingest"
	storeQueriesImportPath = "github.com/similarityyoung/simiclaw/internal/store/queries"
	storeTxImportPath      = "github.com/similarityyoung/simiclaw/internal/store/tx"
	apiImportPath          = "github.com/similarityyoung/simiclaw/pkg/api"
	runtimeImportPath      = "github.com/similarityyoung/simiclaw/internal/runtime"
	runtimeModelPath       = "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func goFilesUnder(t *testing.T, root string, dirs ...string) []string {
	t.Helper()
	var files []string
	for _, dir := range dirs {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	slices.Sort(files)
	return files
}

func fileImportsPath(t *testing.T, absPath string, importPath string) bool {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", absPath, err)
	}
	for _, spec := range parsed.Imports {
		if strings.Trim(spec.Path.Value, `"`) == importPath {
			return true
		}
	}
	return false
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func assertNoStoreDBReference(t *testing.T, dir string) {
	t.Helper()
	root := repoRoot(t)
	files := goFilesUnder(t, root, dir)
	if len(files) == 0 {
		t.Fatalf("expected production files under %s", dir)
	}

	var violations []string
	for _, rel := range files {
		abs := filepath.Join(root, rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.Contains(string(src), "*store.DB") {
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("%s production code must not reference *store.DB:\n%s", dir, strings.Join(violations, "\n"))
	}
}

func assertNoStoreDBReferenceInFiles(t *testing.T, relPaths ...string) {
	t.Helper()
	root := repoRoot(t)
	var violations []string
	for _, rel := range relPaths {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.Contains(string(src), "*store.DB") {
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("production code must not reference *store.DB:\n%s", strings.Join(violations, "\n"))
	}
}

func assertNoPackageImport(t *testing.T, importPath string, dir string) {
	t.Helper()
	root := repoRoot(t)
	files := goFilesUnder(t, root, dir)
	if len(files) == 0 {
		t.Fatalf("expected production files under %s", dir)
	}

	var violations []string
	for _, rel := range files {
		if fileImportsPath(t, filepath.Join(root, rel), importPath) {
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("%s production code must not import %s:\n%s", dir, importPath, strings.Join(violations, "\n"))
	}
}

func assertOnlyAllowedProductionFilesImport(t *testing.T, importPath string, allowed map[string]struct{}) {
	t.Helper()
	root := repoRoot(t)
	files := goFilesUnder(t, root, "cmd", "internal", "pkg")

	var violations []string
	for _, rel := range files {
		if strings.HasPrefix(rel, "internal/store/") {
			continue
		}
		if _, ok := allowed[rel]; ok {
			continue
		}
		if fileImportsPath(t, filepath.Join(root, rel), importPath) {
			violations = append(violations, rel)
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("production code outside allowed files must not import %s:\n%s", importPath, strings.Join(violations, "\n"))
	}
}

func assertNoExportedImportSelectors(t *testing.T, importPath string, dir string) {
	t.Helper()
	root := repoRoot(t)
	files := goFilesUnder(t, root, dir)
	if len(files) == 0 {
		t.Fatalf("expected production files under %s", dir)
	}

	var violations []string
	for _, rel := range files {
		abs := filepath.Join(root, rel)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, abs, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}

		aliases := importAliasesForPath(parsed, importPath)
		if len(aliases) == 0 {
			continue
		}

		for _, decl := range parsed.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if hits := selectorHitsForFuncDecl(fset, d, aliases); len(hits) > 0 {
					for _, hit := range hits {
						violations = append(violations, rel+":"+itoa(hit)+" "+d.Name.Name)
					}
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !typeSpec.Name.IsExported() {
						continue
					}
					if hits := selectorHitsForNode(fset, typeSpec.Type, aliases); len(hits) > 0 {
						for _, hit := range hits {
							violations = append(violations, rel+":"+itoa(hit)+" "+typeSpec.Name.Name)
						}
					}
				}
			}
		}
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("%s exported API must not expose selectors from %s:\n%s", dir, importPath, strings.Join(violations, "\n"))
	}
}

func importAliasesForPath(file *ast.File, importPath string) []string {
	aliases := make([]string, 0, 1)
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != importPath {
			continue
		}
		if spec.Name != nil && spec.Name.Name != "" && spec.Name.Name != "." {
			aliases = append(aliases, spec.Name.Name)
			continue
		}
		parts := strings.Split(importPath, "/")
		aliases = append(aliases, parts[len(parts)-1])
	}
	return aliases
}

func selectorHitsForFuncDecl(fset *token.FileSet, decl *ast.FuncDecl, aliases []string) []int {
	var hits []int
	if decl.Recv != nil {
		hits = append(hits, selectorHitsForNode(fset, decl.Recv, aliases)...)
	}
	hits = append(hits, selectorHitsForNode(fset, decl.Type, aliases)...)
	return hits
}

func selectorHitsForNode(fset *token.FileSet, node ast.Node, aliases []string) []int {
	if node == nil {
		return nil
	}
	var hits []int
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !slices.Contains(aliases, ident.Name) {
			return true
		}
		hits = append(hits, fset.Position(sel.Pos()).Line)
		return true
	})
	return hits
}
