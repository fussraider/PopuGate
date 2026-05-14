package main

import (
	"fmt"
	"os"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/logger"
)

func init() {
	// Propagate build-time values to model package as early as possible,
	// so all subcommands (version, server, …) see the correct values.
	if version != "" {
		model.SetVersion(version)
	}
	if commit != "" {
		model.Commit = commit
	}
}

func main() {
	logger.Init()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
