package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/similarityyoung/simiclaw/internal/guardrails"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: ci-guardrails <check|baseline>")
	}
	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "baseline":
		runBaseline(os.Args[2:])
	default:
		fatal("unknown subcommand %q", os.Args[1])
	}
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	scope := fs.String("scope", string(guardrails.ScopeRepo), "scan scope: repo or pr")
	base := fs.String("base", "", "base git SHA for pr scope")
	head := fs.String("head", "", "head git SHA for pr scope")
	baseline := fs.String("baseline", filepath.ToSlash(".github/guardrails/baseline.json"), "baseline file")
	allowlist := fs.String("allowlist", filepath.ToSlash(".github/guardrails/allowlist.yaml"), "allowlist file")
	jsonOut := fs.String("json", "", "optional JSON report output")
	fs.Parse(args)

	report, err := guardrails.Check(context.Background(), guardrails.CheckConfig{
		Root:          *root,
		Scope:         guardrails.Scope(*scope),
		BaseSHA:       *base,
		HeadSHA:       *head,
		BaselinePath:  *baseline,
		AllowlistPath: *allowlist,
	})
	if err != nil {
		fatal("%v", err)
	}
	if *jsonOut != "" {
		if err := guardrails.WriteReport(*jsonOut, report); err != nil {
			fatal("%v", err)
		}
	}
	if err := printSummary(report); err != nil {
		fatal("%v", err)
	}
	if report.Summary.New > 0 && report.Scope == guardrails.ScopePR {
		os.Exit(1)
	}
}

func runBaseline(args []string) {
	if len(args) == 0 || args[0] != "sync" {
		fatal("usage: ci-guardrails baseline sync --report <file> --baseline <file>")
	}
	fs := flag.NewFlagSet("baseline sync", flag.ExitOnError)
	reportPath := fs.String("report", "", "guardrails report JSON")
	baselinePath := fs.String("baseline", filepath.ToSlash(".github/guardrails/baseline.json"), "baseline output")
	fs.Parse(args[1:])
	if *reportPath == "" {
		fatal("missing --report")
	}
	report, err := guardrails.LoadReport(*reportPath)
	if err != nil {
		fatal("%v", err)
	}
	if err := guardrails.WriteBaseline(*baselinePath, guardrails.BuildBaseline(report)); err != nil {
		fatal("%v", err)
	}
}

func printSummary(report guardrails.Report) error {
	fmt.Printf("scope=%s new=%d existing=%d shrink=%d warnings=%d\n", report.Scope, report.Summary.New, report.Summary.Existing, report.Summary.ShrinkCandidates, report.Summary.Warnings)
	for _, finding := range report.Findings {
		fmt.Printf("%s %s %s:%d %s\n", finding.Status, finding.RuleID, finding.File, finding.StartLine, finding.Message)
	}
	return nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
