package main

import (
	"fmt"
	"os"

	"github.com/wckdboy/wckd-gpu/cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
