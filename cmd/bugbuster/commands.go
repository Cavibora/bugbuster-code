package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// scanCmd — subcommand for project scanning
func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan [path]",
		Short: i18n.T("cli_subcommands.scan_short"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			absDir, _ := filepath.Abs(dir)
			color.Cyan("%s", i18n.T("cli_scan.scanning", absDir))
			cfg := loadConfig()
			p, err := createProvider(cfg)
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			loop := createAgentLoop(cfg, p, nil)
			result, err := loop.Run(fmt.Sprintf(i18n.T("cli_subcommands.scan_prompt"), absDir))
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			fmt.Println(result)
			return nil
		},
	}
}

// fixCmd — subcommand for bug fixing
func fixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix [bug description]",
		Short: i18n.T("cli_subcommands.fix_short"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := strings.Join(args, " ")
			cfg := loadConfig()
			p, err := createProvider(cfg)
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			loop := createAgentLoop(cfg, p, nil)
			result, err := loop.Run(fmt.Sprintf(i18n.T("cli_subcommands.fix_prompt"), description))
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			fmt.Println(result)
			return nil
		},
	}
}

// testCmd — subcommand for running tests
func testCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [path]",
		Short: i18n.T("cli_subcommands.test_short"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			cfg := loadConfig()
			p, err := createProvider(cfg)
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			loop := createAgentLoop(cfg, p, nil)
			result, err := loop.Run(fmt.Sprintf(i18n.T("cli_subcommands.test_prompt"), dir))
			if err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			fmt.Println(result)
			return nil
		},
	}
}

// configCmd — subcommand for configuration management
func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("cli_subcommands.config_short"),
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: i18n.T("cli_subcommands.config_show_short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			data, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: i18n.T("cli_subcommands.config_init_short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			dir, _ := os.Getwd()
			// Use visible config file (bugbuster.yaml), hidden (.bugbuster.yaml) also works
			configPath := filepath.Join(dir, "bugbuster.yaml")
			if err := cfg.SaveConfig(configPath); err != nil {
				color.Red("%s", i18n.T("cli_error.general", err))
				return err
			}
			color.Green("%s", i18n.T("cli_success.config_created", configPath))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "providers",
		Short: i18n.T("cli_subcommands.config_providers_short"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			color.Cyan("%s", i18n.T("cli_config.providers_header"))
			for name, prov := range cfg.Providers {
				active := ""
				if name == cfg.DefaultProvider {
					active = i18n.T("cli_config.default_marker")
				}
				color.Green("  - %s: type=%s model=%s%s", name, prov.Type, prov.Model, active)
			}
			return nil
		},
	})

	return cmd
}

// versionCmd — subcommand for version
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: i18n.T("cli_subcommands.version_short"),
		Run: func(cmd *cobra.Command, args []string) {
			color.Cyan("BugBuster Code %s", Version)
			color.Yellow("  Commit: %s", GitCommit)
			color.Yellow("  Built:  %s", BuildDate)
		},
	}
}