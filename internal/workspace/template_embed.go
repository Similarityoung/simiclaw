package workspace

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed templates/*.md
var templateFS embed.FS

var templates = mustLoadTemplates()

func mustLoadTemplates() map[string]string {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		panic(fmt.Sprintf("workspace: read embedded templates: %v", err))
	}
	loaded := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.ToSlash(filepath.Join("templates", entry.Name()))
		data, err := templateFS.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("workspace: read embedded template %s: %v", path, err))
		}
		loaded[entry.Name()] = strings.TrimSpace(string(data)) + "\n"
	}
	return loaded
}

func templateNames() []string {
	paths := make([]string, 0, len(templates))
	for path := range templates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
