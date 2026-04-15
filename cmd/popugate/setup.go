package main

import (
	"context"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fussraider/PopuGate/pkg/logger"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/fussraider/PopuGate/internal/database"
	"github.com/fussraider/PopuGate/internal/store"
)

// setupCmd represents the setup command.
var setupCmd = &cobra.Command{
	Use:   "setup [password]",
	Short: "Set the initial admin password (first run only)",
	Long: `Set the initial admin password for the PopuGate web interface.
This command can only be run once — when no password has been set yet.

Usage:
  popugate setup                  (interactive prompt)
  popugate setup MyP@ssw0rd       (direct password)
  popugate setup --data /path     (custom data directory)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringP("data", "d", "", "Data directory (default: auto-detect)")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data")
	if dataDir == "" {
		dataDir = resolveDataDir()
	}

	dbPath := filepath.Join(dataDir, "settings.db")

	// Open database
	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	settingsStore := store.NewSettingsStore(db)

	ctx := context.Background()
	hash, err := settingsStore.GetAuthPasswordHash(ctx)
	if err != nil {
		return fmt.Errorf("read password hash: %w", err)
	}

	if hash != "" {
		return fmt.Errorf("setup already completed — password is already set. Use the web UI to change it")
	}

	// Get password
	var password string
	if len(args) > 0 {
		password = args[0]
	} else {
		password, err = readPasswordInteractive()
		if err != nil {
			return err
		}
	}

	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}

	// Hash and store
	newHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := settingsStore.SetAuthPasswordHash(ctx, string(newHash)); err != nil {
		return fmt.Errorf("save password hash: %w", err)
	}

	logger.WithScope("setup").Infof("Admin password set successfully. You can now access the web UI.")
	return nil
}

func readPasswordInteractive() (string, error) {
	fmt.Print("Enter admin password (min 6 characters): ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	fmt.Print("Confirm admin password: ")
	confirm, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read confirmation: %w", err)
	}

	if string(password) != string(confirm) {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(password), nil
}
