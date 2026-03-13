package guardrails

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

type funcRange struct {
	start  token.Pos
	end    token.Pos
	symbol string
}

func collectFuncRanges(file *ast.File) []funcRange {
	var ranges []funcRange
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		symbol := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			symbol = methodSymbol(fn)
		}
		ranges = append(ranges, funcRange{
			start:  fn.Pos(),
			end:    fn.End(),
			symbol: symbol,
		})
	}
	return ranges
}

func methodSymbol(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	switch expr := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return "(*" + ident.Name + ")." + fn.Name.Name
		}
	case *ast.Ident:
		return expr.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func callFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange, scope Scope, change changedFile) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		line := fset.Position(call.Pos()).Line
		if scope == ScopePR && !change.contains(line) {
			return true
		}
		symbol := symbolForLine(call.Pos(), funcs)
		sourceLine := lineText(lines, line)
		switch pkgIdent.Name + "." + selector.Sel.Name {
		case "context.Background", "context.TODO":
			findings = append(findings, newLineFinding("context-background", "error", rel, line, symbol, sourceLine, fmt.Sprintf("avoid new %s() in production code", selector.Sel.Name), "Background contexts hide cancellation and timeout boundaries in production code.", "Thread an existing context into this call or document a reviewed entrypoint exception in the allowlist."))
		case "os.WriteFile", "ioutil.WriteFile":
			findings = append(findings, newLineFinding("writefile", "error", rel, line, symbol, sourceLine, "avoid new direct WriteFile calls in production code", "Direct file writes bypass the repository's explicit workspace and persistence boundaries.", "Route the write through the existing owner boundary or add a reviewed allowlist entry for a true boundary owner."))
		default:
			if (pkgIdent.Name == "fmt" || pkgIdent.Name == "log") && strings.HasPrefix(selector.Sel.Name, "Print") {
				findings = append(findings, newLineFinding("print-logging", "error", rel, line, symbol, sourceLine, fmt.Sprintf("avoid new %s.%s calls in production code", pkgIdent.Name, selector.Sel.Name), "Ad hoc printing bypasses structured logging and makes operational diagnostics inconsistent.", "Use the repository's structured logging helpers instead of direct fmt/log printing."))
			}
		}
		return true
	})
	return findings
}

func goRoutineFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		stmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		line := fset.Position(stmt.Pos()).Line
		symbol := symbolForLine(stmt.Pos(), funcs)
		findings = append(findings, newLineFinding("go-statement", "warning", rel, line, symbol, lineText(lines, line), "review go statement ownership and shutdown behavior", "Background goroutines are a common source of hidden lifecycle and reliability drift.", "Confirm the goroutine has an owner, a stop path, and panic handling."))
		return true
	})
	return findings
}

func panicFindings(fset *token.FileSet, file *ast.File, rel string, lines []string, funcs []funcRange) []Finding {
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "panic" {
			return true
		}
		line := fset.Position(call.Pos()).Line
		symbol := symbolForLine(call.Pos(), funcs)
		findings = append(findings, newLineFinding("panic-call", "warning", rel, line, symbol, lineText(lines, line), "review panic usage in production code", "Panics in production paths can turn recoverable failures into process-wide incidents.", "Prefer explicit error propagation unless this is a deliberately fatal bootstrap path."))
		return true
	})
	return findings
}

func symbolForLine(pos token.Pos, funcs []funcRange) string {
	for _, fn := range funcs {
		if pos >= fn.start && pos <= fn.end {
			return fn.symbol
		}
	}
	return ""
}
