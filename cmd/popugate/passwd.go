package main

import (
	"context"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fussraider/PopuGate/internal/database"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var passwdCmd = &cobra.Command{
	Use:   "passwd [password]",
	Short: "Change the admin password",
	Long: `Change the admin password for the PopuGate web interface.

Usage:
  popugate passwd                  (interactive prompt)
  popugate passwd MyN3wP@ss        (direct password)
  popugate passwd --data /path     (custom data directory)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPasswd,
}

func init() {
	passwdCmd.Flags().StringP("data", "d", "", "Data directory (default: auto-detect)")
	rootCmd.AddCommand(passwdCmd)
}

func runPasswd(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data")
	if dataDir == "" {
		dataDir = resolveDataDir()
	}

	dbPath := filepath.Join(dataDir, "settings.db")

	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = database.Close() }()

	settingsStore := store.NewSettingsStore(db)

	ctx := context.Background()
	hash, err := settingsStore.GetAuthPasswordHash(ctx)
	if err != nil {
		return fmt.Errorf("read password hash: %w", err)
	}

	if hash == "" {
		return fmt.Errorf("no password set yet — run 'popugate setup' first")
	}

	var password string
	if len(args) > 0 {
		password = args[0]
	} else {
		password, err = readPasswdInteractive()
		if err != nil {
			return err
		}
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := settingsStore.SetAuthPasswordHash(ctx, string(newHash)); err != nil {
		return fmt.Errorf("save password hash: %w", err)
	}

	logger.WithScope("passwd").Infof("Admin password changed successfully.")
	return nil
}

func readPasswdInteractive() (string, error) {
	fmt.Print("Enter new admin password (min 8 characters): ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	fmt.Print("Confirm new admin password: ")
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
