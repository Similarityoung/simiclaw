package hygiene

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type SkillDoc struct {
	Path        string
	Name        string
	Description string
}

func ValidateSkills(root string) ([]SkillDoc, error) {
	var paths []string
	for _, dir := range []string{"skills", filepath.Join("workspace", "skills")} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() || filepath.Base(path) != "SKILL.md" {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)

	var docs []SkillDoc
	for _, path := range paths {
		doc, err := parseSkillDoc(root, path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func parseSkillDoc(root, absPath string) (SkillDoc, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return SkillDoc{}, err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return SkillDoc{}, fmt.Errorf("%s: missing YAML frontmatter", absPath)
	}
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) != 2 {
		return SkillDoc{}, fmt.Errorf("%s: invalid YAML frontmatter fence", absPath)
	}
	fields := map[string]string{}
	seen := map[string]struct{}{}
	lines := strings.Split(strings.TrimPrefix(parts[0], "---\n"), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		chunks := strings.SplitN(line, ":", 2)
		if len(chunks) != 2 {
			return SkillDoc{}, fmt.Errorf("%s: invalid frontmatter line %q", absPath, raw)
		}
		key := strings.TrimSpace(chunks[0])
		value := strings.TrimSpace(chunks[1])
		value = strings.Trim(value, `"'`)
		if _, ok := seen[key]; ok {
			return SkillDoc{}, fmt.Errorf("%s: duplicated frontmatter key %q", absPath, key)
		}
		seen[key] = struct{}{}
		fields[key] = value
	}

	name := strings.TrimSpace(fields["name"])
	description := strings.TrimSpace(fields["description"])
	if name == "" {
		return SkillDoc{}, fmt.Errorf("%s: missing frontmatter name", absPath)
	}
	if description == "" {
		return SkillDoc{}, fmt.Errorf("%s: missing frontmatter description", absPath)
	}

	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return SkillDoc{}, err
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "skills/") && !isHyphenCase(name) {
		return SkillDoc{}, fmt.Errorf("%s: repo skill name %q must be hyphen-case", rel, name)
	}

	return SkillDoc{
		Path:        rel,
		Name:        name,
		Description: description,
	}, nil
}

func isHyphenCase(value string) bool {
	if value == "" {
		return false
	}
	lastHyphen := false
	for i, r := range value {
		switch {
		case unicode.IsLower(r) || unicode.IsDigit(r):
			lastHyphen = false
		case r == '-':
			if i == 0 || lastHyphen {
				return false
			}
			lastHyphen = true
		default:
			return false
		}
	}
	return !lastHyphen
}
