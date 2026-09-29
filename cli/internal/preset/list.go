package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// List reads every *.yaml preset in dir, in name order.
// A missing directory is an error. An empty directory returns no presets.
func List(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("presets dir %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]File, 0, len(names))
	for _, name := range names {
		f, err := LoadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}
