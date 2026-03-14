package guardrails

import (
	"fmt"
	"strconv"
	"strings"
)

type changedFile struct {
	path   string
	ranges []lineRange
}

type lineRange struct {
	start int
	end   int
}

func (r lineRange) contains(line int) bool {
	return line >= r.start && line <= r.end
}

func (f changedFile) contains(line int) bool {
	for _, r := range f.ranges {
		if r.contains(line) {
			return true
		}
	}
	return false
}

func parseDiff(diff string) (map[string]changedFile, error) {
	changes := map[string]changedFile{}
	var current string

	for _, raw := range strings.Split(diff, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "+++ b/") {
			current = strings.TrimPrefix(line, "+++ b/")
			current = strings.TrimPrefix(current, "./")
			if _, ok := changes[current]; !ok {
				changes[current] = changedFile{path: current}
			}
			continue
		}
		if !strings.HasPrefix(line, "@@") || current == "" {
			continue
		}
		start, count, err := parseNewRange(line)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			continue
		}
		entry := changes[current]
		entry.ranges = append(entry.ranges, lineRange{
			start: start,
			end:   start + count - 1,
		})
		changes[current] = entry
	}

	return changes, nil
}

func parseNewRange(header string) (int, int, error) {
	parts := strings.Split(header, " ")
	for _, part := range parts {
		if !strings.HasPrefix(part, "+") {
			continue
		}
		segment := strings.TrimPrefix(part, "+")
		chunks := strings.SplitN(segment, ",", 2)
		start, err := strconv.Atoi(strings.TrimSpace(chunks[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("parse diff start %q: %w", header, err)
		}
		count := 1
		if len(chunks) == 2 {
			count, err = strconv.Atoi(strings.TrimSpace(chunks[1]))
			if err != nil {
				return 0, 0, fmt.Errorf("parse diff count %q: %w", header, err)
			}
		}
		return start, count, nil
	}
	return 0, 0, fmt.Errorf("missing new range in %q", header)
}
