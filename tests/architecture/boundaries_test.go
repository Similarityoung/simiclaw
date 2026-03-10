package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestOnlyIngestServiceCallsIngestEventOutsideTests(t *testing.T) {
	root := repoRoot(t)
	files := goFilesUnder(t, root, "cmd", "internal")
	allowed := "internal/ingest/service.go"
	var violations []string

	for _, rel := range files {
		if rel == allowed {
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
			if !ok || selector.Sel == nil || selector.Sel.Name != "IngestEvent" {
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
	t.Fatalf("found direct IngestEvent calls outside %s:\n%s", allowed, strings.Join(violations, "\n"))
}

func TestHTTPListHandlersImportQueryServiceInsteadOfStore(t *testing.T) {
	root := repoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "internal/httpapi/handlers_*_query.go"))
	if err != nil {
		t.Fatalf("glob query handlers: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected query handlers")
	}

	const (
		queryImport = "github.com/similarityyoung/simiclaw/internal/query"
		storeImport = "github.com/similarityyoung/simiclaw/internal/store"
	)

	for _, abs := range matches {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			t.Fatalf("relative path for %s: %v", abs, err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, abs, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", rel, err)
		}

		var imports []string
		for _, spec := range parsed.Imports {
			imports = append(imports, strings.Trim(spec.Path.Value, `"`))
		}
		if !slices.Contains(imports, queryImport) {
			t.Fatalf("%s must import %s", filepath.ToSlash(rel), queryImport)
		}
		if slices.Contains(imports, storeImport) {
			t.Fatalf("%s must not import %s", filepath.ToSlash(rel), storeImport)
		}
	}
}

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
