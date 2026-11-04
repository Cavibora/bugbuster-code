package main

import (
	"testing"

	"bugbuster-code/pkg/config"

	"github.com/spf13/cobra"
)

// newServeTestCmd returns a serve command with the --auto-continue flag wired to
// the package-level serveAutoContinue var, resetting the var for isolation.
func newServeTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	serveAutoContinue = false
	cmd := &cobra.Command{Use: "serve"}
	cmd.Flags().BoolVar(&serveAutoContinue, "auto-continue", false, "test flag")
	return cmd
}

func TestServeAutoContinueValue(t *testing.T) {
	yes, no := true, false

	cases := []struct {
		name     string
		flagSet  bool // simulate --auto-continue passed on the command line
		flagVal  bool
		cfgValue *bool // serve.auto_continue in config (nil = unset)
		want     bool
	}{
		{"default off", false, false, nil, false},
		{"config on", false, false, &yes, true},
		{"config off", false, false, &no, false},
		{"flag on beats config off", true, true, &no, true},
		{"flag off beats config on", true, false, &yes, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newServeTestCmd(t)
			cfg := &config.BugBusterConfig{}
			cfg.Serve.AutoContinue = tc.cfgValue

			if tc.flagSet {
				serveAutoContinue = tc.flagVal
				if err := cmd.Flags().Set("auto-continue", boolToStr(tc.flagVal)); err != nil {
					t.Fatalf("set flag: %v", err)
				}
			}

			if got := serveAutoContinueValue(cmd, cfg); got != tc.want {
				t.Errorf("serveAutoContinueValue = %v, want %v", got, tc.want)
			}
		})
	}
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
