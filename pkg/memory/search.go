package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SearchArgs struct {
	Query       string
	Scope       string
	TopK        int
	ChannelType string
}

type SearchHit struct {
	Path    string  `json:"path"`
	Scope   string  `json:"scope"`
	Lines   []int   `json:"lines"`
	Score   float64 `json:"score"`
	Preview string  `json:"preview"`
}

type SearchResult struct {
	Disabled bool        `json:"disabled"`
	Hits     []SearchHit `json:"hits"`
}

func Search(workspace string, args SearchArgs) (SearchResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("query is required")
	}

	topK := args.TopK
	if topK <= 0 {
		topK = 6
	}
	if topK > 20 {
		topK = 20
	}

	allowedScopes, err := resolveScopes(strings.TrimSpace(args.Scope), strings.TrimSpace(args.ChannelType))
	if err != nil {
		return SearchResult{}, err
	}

	files, err := ListFiles(workspace)
	if err != nil {
		return SearchResult{}, err
	}
	if len(files) == 0 {
		return SearchResult{Disabled: false, Hits: nil}, nil
	}

	needle := strings.ToLower(query)
	tokens := tokenize(query)
	candidates := make([]SearchHit, 0, 32)
	for _, f := range files {
		if !allowedScopes[f.Scope] {
			continue
		}
		abs := filepath.Join(workspace, filepath.FromSlash(f.Path))
		fh, err := os.Open(abs)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(fh)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			score := scoreLine(strings.ToLower(line), needle, tokens)
			if score <= 0 {
				continue
			}
			preview := truncatePreview(line, 120)
			candidates = append(candidates, SearchHit{
				Path:    f.Path,
				Scope:   f.Scope,
				Lines:   []int{lineNo, lineNo},
				Score:   float64(score),
				Preview: preview,
			})
		}
		_ = fh.Close()
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Path == candidates[j].Path {
				return candidates[i].Lines[0] < candidates[j].Lines[0]
			}
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return SearchResult{Disabled: false, Hits: candidates}, nil
}

func resolveScopes(scope, channelType string) (map[string]bool, error) {
	switch scope {
	case "", "auto":
		return allowedScopesForChannel(channelType), nil
	case "private":
		return map[string]bool{"private": true}, nil
	case "public":
		return map[string]bool{"public": true}, nil
	default:
		return nil, fmt.Errorf("invalid scope")
	}
}

func tokenize(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	if len(parts) == 0 {
		return []string{strings.ToLower(query)}
	}
	return parts
}

func scoreLine(line, full string, tokens []string) int {
	score := 0
	if strings.Contains(line, full) {
		score += 3
	}
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		if strings.Contains(line, tok) {
			score++
		}
	}
	return score
}

func truncatePreview(line string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= maxRunes {
		return line
	}
	return string(runes[:maxRunes]) + "..."
}
