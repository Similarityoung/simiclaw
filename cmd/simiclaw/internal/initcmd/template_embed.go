package initcmd

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed templates/*.md
var workspaceTemplateFS embed.FS

var workspaceTemplates = mustLoadWorkspaceTemplates()

func mustLoadWorkspaceTemplates() map[string]string {
	entries, err := workspaceTemplateFS.ReadDir("templates")
	if err != nil {
		panic(fmt.Sprintf("initcmd: read embedded templates: %v", err))
	}
	loaded := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.ToSlash(filepath.Join("templates", entry.Name()))
		data, err := workspaceTemplateFS.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("initcmd: read embedded template %s: %v", path, err))
		}
		loaded[entry.Name()] = strings.TrimSpace(string(data)) + "\n"
	}
	return loaded
}

func sortedWorkspaceTemplateNames() []string {
	paths := make([]string, 0, len(workspaceTemplates))
	for path := range workspaceTemplates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
