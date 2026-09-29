//go:build !unix

package cli

import (
	"fmt"
	"path/filepath"
)

func spawnSweeper(string, string) (int, error) {
	return 0, fmt.Errorf("background sweeper is supported on unix; run `wckd sweeper` in another terminal")
}

func pidPath(stateDir string) string {
	return filepath.Join(stateDir, "sweeper.pid")
}
