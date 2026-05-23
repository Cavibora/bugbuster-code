package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/i18n"
)

const (
	// autoMaxIterations — maximum count iterations autopilot default.
	autoMaxIterations = 50
	// autoDelayBetweenIterations — delay between autopilot iterations.
	autoDelayBetweenIterations = 2 * time.Second
)

// AutoPilotState stores autopilot mode state.
type AutoPilotState struct {
	Enabled     bool
	Iteration   int
	MaxIterations int
}

// NewAutoPilotState creates autopilot state with iteration limit.
func NewAutoPilotState(maxIterations int) *AutoPilotState {
	if maxIterations <= 0 {
		maxIterations = autoMaxIterations
	}
	return &AutoPilotState{
		MaxIterations: maxIterations,
	}
}

// isPlanCompleted checks if text contains plan completion indicators.
// Checks last 500 characters of assistant messages.
// Markers are loaded from i18n — ALL languages are checked concurrently,
// to correctly detect completion regardless of agent response language.
func isPlanCompleted(text string) bool {
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