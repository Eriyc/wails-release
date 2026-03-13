package main

import (
	"os"

	"github.com/you/wailsrel/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
