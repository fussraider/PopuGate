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
		fmt.Printf("PopuGate v%s\n", model.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
