package main

import (
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of PopuGate",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("PopuGate %s\n", model.Version)
	if model.Commit != "unknown" && model.Commit != "" {
		fmt.Printf("Commit: %s\n", model.Commit)
	}
	fmt.Println(model.VersionURL())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
