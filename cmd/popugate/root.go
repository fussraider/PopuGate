package main

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "popugate",
	Short: "PopuGate MTProto Proxy Manager",
	Long: `PopuGate — a modern MTProto proxy manager with a web UI and REST API.

Usage:
  popugate [command]

Available Commands:
  server      Start the HTTP API server and web UI
  setup       Set the initial admin password (first run only)
  passwd      Change the admin password
  help        Help about any command`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}
