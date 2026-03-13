package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/similarityyoung/simiclaw/devtools/hygiene"
)

func main() {
	fs := flag.NewFlagSet("hygiene-docs", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	report := fs.String("guardrails-report", "", "guardrails report JSON")
	quality := fs.String("quality-doc", "", "quality score doc path")
	debt := fs.String("tech-debt-doc", "", "tech debt doc path")
	linksStatus := fs.String("links-status", "unknown", "docs links status")
	linksReport := fs.String("links-report", "", "docs links report path")
	prBody := fs.String("pr-body", "", "optional PR body output path")
	fs.Parse(os.Args[1:])
	if *report == "" {
		fmt.Fprintln(os.Stderr, "missing --guardrails-report")
		os.Exit(1)
	}
	if err := hygiene.UpdateDocs(hygiene.UpdateDocsConfig{
		Root:             *root,
		GuardrailsReport: *report,
		QualityDoc:       *quality,
		TechDebtDoc:      *debt,
		LinksStatus:      *linksStatus,
		LinksReport:      *linksReport,
		PRBody:           *prBody,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
