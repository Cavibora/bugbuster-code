package main

import (
	"fmt"
	"os"

	"bugbuster-code/pkg/i18n"

	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	verbose      bool
	debug        bool
	model        string
	projectDir   string
	permissionMode string
	sessionID    string
	sessionName   string
	langFlag     string
	tuiMode      string // "auto", "inline", "" (no TUI)
	clearCrash   bool
)

func main() {
	// Initialize i18n with default language (will be updated in runInteractive)
	i18n.Init("en")

	// Setup crash handler — redirect stderr to crash log file
	// This ensures that any panic or runtime.throw output goes to the file
	// instead of the terminal, and we can show a friendly message
	crashCleanup, prevCrashPath := setupCrashHandler()
	if prevCrashPath != "" {
		showPreviousCrashNotification(prevCrashPath)
	}
	defer crashCleanup()

	// Install global panic handler — catches panics in main goroutine
	defer func() {
		if r := recover(); r != nil {
			writeCrashLog(r)
		}
	}()

	rootCmd := &cobra.Command{
		Use:   i18n.T("cli_subcommands.scan_use"),
		Short: i18n.T("cli.short_desc"),
		Long:  i18n.T("cli.long_desc"),
		Args:  cobra.ArbitraryArgs,
		Run:   runInteractive,
	}

	// Subcommands
	rootCmd.AddCommand(scanCmd())
	rootCmd.AddCommand(fixCmd())
	rootCmd.AddCommand(testCmd())
	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(execCmd())
	rootCmd.AddCommand(mcpServeCmd())

	// Flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", i18n.T("cli_flag.config"))
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, i18n.T("cli_flag.verbose"))
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "D", false, i18n.T("cli_flag.debug"))
	rootCmd.PersistentFlags().StringVarP(&model, "model", "m", "", i18n.T("cli_flag.model"))
	rootCmd.PersistentFlags().StringVarP(&projectDir, "dir", "d", "", i18n.T("cli_flag.dir"))
	rootCmd.PersistentFlags().StringVarP(&permissionMode, "permission-mode", "p", "", i18n.T("cli_flag.permission_mode"))
	rootCmd.PersistentFlags().StringVarP(&sessionID, "session", "s", "", i18n.T("cli_flag.session"))
	rootCmd.PersistentFlags().StringVarP(&sessionName, "session-name", "n", "", i18n.T("cli_flag.session_name"))
	rootCmd.PersistentFlags().StringVarP(&langFlag, "lang", "l", "", i18n.T("cli_flag.lang"))
	rootCmd.PersistentFlags().StringVarP(&tuiMode, "tui", "t", "", "TUI mode: auto (AltScreen) or inline")
	rootCmd.PersistentFlags().BoolVar(&clearCrash, "clear-crash", false, "Clear crash logs and dismiss notification")

	// Handle --clear-crash before anything else
	if clearCrash {
		clearCrashLogs()
		fmt.Println("Crash logs cleared.")
		os.Exit(0)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
