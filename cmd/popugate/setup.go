package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fussraider/PopuGate/internal/database"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
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
	setupCmd.Flags().Bool("enable-tcp-tune", false, "Enable Linux kernel TCP optimizations (BBR & FastOpen)")
	setupCmd.Flags().Bool("disable-tcp-tune", false, "Disable Linux kernel TCP optimizations (BBR & FastOpen)")
	rootCmd.AddCommand(setupCmd)
}

func handlePasswordSetup(ctx context.Context, settingsStore *store.SettingsStore, args []string) error {
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

func handleSysctlSetup(ctx context.Context, settingsStore *store.SettingsStore, cmd *cobra.Command, args []string) error {
	// TCP Optimizations stateful management
	enableTuning, _ := cmd.Flags().GetBool("enable-tcp-tune")
	disableTuning, _ := cmd.Flags().GetBool("disable-tcp-tune")

	var runTuning bool
	var tuneEnabled bool

	if enableTuning {
		runTuning = true
		tuneEnabled = true
	} else if disableTuning {
		runTuning = true
		tuneEnabled = false
	} else if len(args) == 0 && runtime.GOOS == "linux" {
		// Only ask interactive prompt on Linux
		fmt.Print("Enable Linux kernel network TCP optimizations (BBR & TCP FastOpen)? [Y/n]: ")
		var answer string
		_, _ = fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "" || answer == "y" || answer == "yes" {
			runTuning = true
			tuneEnabled = true
		}
	}

	if !runTuning {
		return nil
	}

	if runtime.GOOS != "linux" {
		logger.WithScope("setup").Warnf("TCP BBR/FastOpen optimizations are only supported on Linux.")
		return nil
	}

	if err := service.ConfigureSysctl(ctx, settingsStore, tuneEnabled); err != nil {
		logger.WithScope("setup").Warnf("Failed to configure TCP optimizations: %v (You may need to run setup with root/sudo privileges)", err)
		return nil
	}

	if tuneEnabled {
		logger.WithScope("setup").Infof("Linux kernel TCP optimizations (BBR & FastOpen) applied successfully.")
	} else {
		logger.WithScope("setup").Infof("TCP optimizations disabled and original system configuration restored.")
	}

	return nil
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
	defer func() { _ = database.Close() }()

	settingsStore := store.NewSettingsStore(db)
	ctx := context.Background()

	if err := handlePasswordSetup(ctx, settingsStore, args); err != nil {
		return err
	}

	return handleSysctlSetup(ctx, settingsStore, cmd, args)
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
