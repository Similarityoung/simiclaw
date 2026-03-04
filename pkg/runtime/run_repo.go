package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

// RunRepo 提供 runtime/runs 目录下 run trace 的读查询能力。
type RunRepo struct {
	dir string
}

func NewRunRepo(workspace string) *RunRepo {
	return &RunRepo{dir: filepath.Join(workspace, "runtime", "runs")}
}

// Get 按 run_id 读取 run trace。
func (r *RunRepo) Get(runID string) (model.RunTrace, bool, error) {
	path := filepath.Join(r.dir, runID+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.RunTrace{}, false, nil
		}
		return model.RunTrace{}, false, err
	}
	var trace model.RunTrace
	if err := json.Unmarshal(b, &trace); err != nil {
		return model.RunTrace{}, false, err
	}
	return trace, true, nil
}

// List 返回 runs 目录下所有可解析的 run trace（按 run_id 升序稳定输出）。
func (r *RunRepo) List() ([]model.RunTrace, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]model.RunTrace, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(r.dir, name))
		if err != nil {
			return nil, err
		}
		var trace model.RunTrace
		if err := json.Unmarshal(b, &trace); err != nil {
			return nil, err
		}
		out = append(out, trace)
	}
	return out, nil
}
