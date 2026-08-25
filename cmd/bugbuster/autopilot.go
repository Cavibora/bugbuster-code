package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
)

const (
	// autoMaxIterations — maximum count iterations autopilot default.
	autoMaxIterations = 5000
	// autoDelayBetweenIterations — delay between autopilot iterations.
	autoDelayBetweenIterations = 2 * time.Second
)

// AutoPilotState stores autopilot mode state.
type AutoPilotState struct {
	Enabled       bool
	Iteration     int
	MaxIterations int
}

// NewAutoPilotState creates autopilot state with iteration limit.
// If maxIterations <= 0, uses the default (autoMaxIterations).
func NewAutoPilotState(maxIterations int) *AutoPilotState {
	if maxIterations <= 0 {
		maxIterations = autoMaxIterations
	}
	return &AutoPilotState{
		MaxIterations: maxIterations,
	}
}

// NewAutoPilotStateFromConfig creates autopilot state with iteration limit
// from the config. If maxIterations <= 0, uses cfg.Agent.Autopilot.MaxIterations.
func NewAutoPilotStateFromConfig(cfg *config.BugBusterConfig, maxIterations int) *AutoPilotState {
	if maxIterations <= 0 {
		maxIterations = cfg.Agent.Autopilot.MaxIterations
	}
	if maxIterations <= 0 {
		maxIterations = autoMaxIterations
	}
	return &AutoPilotState{
		MaxIterations: maxIterations,
	}
}

// autoDelay returns the delay between autopilot iterations from config.
func autoDelay(cfg *config.BugBusterConfig) time.Duration {
	if cfg != nil && cfg.Agent.Autopilot.DelayMs > 0 {
		return time.Duration(cfg.Agent.Autopilot.DelayMs) * time.Millisecond
	}
	return autoDelayBetweenIterations
}

// isPlanCompleted checks if text contains plan completion indicators.
// Also checks for recap/summary markers (※ Recap:, Recap:, Итог:, Summary:).
func isPlanCompleted(text string) bool {
	// First check recap markers — these are strong completion signals
	if agent.LooksLikeCompletion(text) {
		return true
	}

	markers := getCompletionMarkers()
	lower := strings.ToLower(text)
	if len(lower) > 500 {
		lower = lower[len(lower)-500:]
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// getCompletionMarkers returns plan completion markers from all languages.
// Markers are stored in i18n key cli.auto_completion_markers (delimiter |).
func getCompletionMarkers() []string {
	allTranslations := i18n.TAll("cli.auto_completion_markers")
	var markers []string
	for _, translation := range allTranslations {
		for _, m := range strings.Split(translation, "|") {
			m = strings.TrimSpace(m)
			if m != "" {
				markers = append(markers, strings.ToLower(m))
			}
		}
	}
	return markers
}

// randomContinuePhrase returns a random phrase to continue work.
// Phrases are loaded from i18n key cli.auto_phrases (delimiter |),
// depends on current language.
func randomContinuePhrase() string {
	phrasesStr := i18n.T("cli.auto_phrases")
	phrases := strings.Split(phrasesStr, "|")
	var filtered []string
	for _, p := range phrases {
		p = strings.TrimSpace(p)
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return "Continue"
	}
	return filtered[rand.Intn(len(filtered))]
}

// getLastAssistantMessage returns text last messages assistant.
// Returns empty line if no assistant messages.
func getLastAssistantMessage(loop *agent.AgentLoop) string {
	msgs := loop.Context.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].GetText()
		}
	}
	return ""
}

// formatAutoIteration formats autopilot iteration line.
func formatAutoIteration(iteration, maxIterations int, phrase string) string {
	return fmt.Sprintf(i18n.T("cli.auto_iteration"), iteration, maxIterations, phrase)
}
