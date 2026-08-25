package main

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
)

func init() {
	// Initialize i18n for tests
	i18n.Init("en")
}

func TestIsPlanCompleted(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Russian — plan completion
		{"все фазы завершены", "Отлично! Все фазы завершены. Проект готов.", true},
		{"все спринты выполнены", "Все спринты выполнены успешно.", true},
		{"все пункты завершены", "Все пункты завершены, можно переходить к тестированию.", true},
		{"все шаги выполнены", "Все шаги выполнены.", true},
		{"все этапы завершены", "Все этапы завершены!", true},
		{"все задачи выполнены", "Все задачи выполнены.", true},
		{"план завершён", "План завершён, все цели достигнуты.", true},
		{"план завершен", "План завершен на 100%.", true},
		{"план выполнен", "План выполнен полностью.", true},
		{"план реализован", "План реализован.", true},
		{"всё готово", "Всё готово, можно деплоить.", true},
		{"всё сделано", "Всё сделано.", true},
		{"последняя фаза завершена", "Последняя фаза завершена.", true},
		{"последний спринт выполнен", "Последний спринт выполнен.", true},
		{"последний этап завершен", "Последний этап завершен.", true},

		// English — plan completion
		{"all phases completed", "All phases completed successfully.", true},
		{"all steps done", "All steps done.", true},
		{"all sprints completed", "All sprints completed.", true},
		{"all tasks done", "All tasks done.", true},
		{"plan completed", "Plan completed.", true},
		{"plan finished", "Plan finished.", true},
		{"plan done", "Plan done.", true},
		{"last phase completed", "Last phase completed.", true},
		{"last step done", "Last step done.", true},
		{"all done", "All done!", true},
		{"all completed", "All completed.", true},
		{"all finished", "All finished.", true},

		// NOT completion — intermediate phases
		{"фаза 1 завершена", "Фаза 1 завершена. Переходим к фазе 2.", false},
		{"шаг 3 выполнен", "Шаг 3 выполнен. Следующий шаг — интеграция.", false},
		{"спринт 2 завершен", "Спринт 2 завершен, начинаем спринт 3.", false},
		{"этап 1 завершен", "Этап 1 завершен.", false},
		{"пункт 2 выполнен", "Пункт 2 выполнен.", false},

		// NOT completion — plain text
		{"просто текст", "Привет, я проанализировал код и нашёл несколько проблем.", false},
		{"empty", "", false},
		{"английский промежуточный", "Phase 1 completed. Moving to phase 2.", false},
		{"step 2 done", "Step 2 done, continuing with step 3.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPlanCompleted(tt.text)
			if result != tt.expected {
				t.Errorf("isPlanCompleted(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestIsPlanCompleted_LongText(t *testing.T) {
	// Verify detector works on last 500 chars of long text
	longPrefix := strings.Repeat("x", 1000) + " "
	completed := longPrefix + "Все фазы завершены."
	notCompleted := "Все фазы завершены." + longPrefix

	if !isPlanCompleted(completed) {
		t.Error("should detect completion at end of long text")
	}
	if isPlanCompleted(notCompleted) {
		t.Error("should NOT detect completion at beginning of long text (outside 500 char window)")
	}
}

func TestIsPlanCompleted_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"All Phases Completed", "All Phases Completed."},
		{"PLAN COMPLETED", "PLAN COMPLETED."},
		{"ВСЁ ГОТОВО", "ВСЁ ГОТОВО."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !isPlanCompleted(tt.text) {
				t.Errorf("isPlanCompleted(%q) should be case-insensitive", tt.text)
			}
		})
	}
}

func TestIsPlanCompleted_Multilingual(t *testing.T) {
	// Verify markers from different languages work simultaneously
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// German markers
		{"de: alle phasen abgeschlossen", "Alle Phasen abgeschlossen.", true},
		{"de: plan beendet", "Plan beendet.", true},
		// Spanish markers
		{"es: todas las fases completadas", "Todas las fases completadas.", true},
		{"es: plan completado", "Plan completado.", true},
		// French markers
		{"fr: toutes les phases terminées", "Toutes les phases terminées.", true},
		{"fr: plan terminé", "Plan terminé.", true},
		// Japanese markers
		{"ja: 全フェーズ完了", "全フェーズ完了。", true},
		// Chinese markers
		{"zh: 所有阶段完成", "所有阶段完成。", true},
		// Portuguese markers
		{"pt: todas as fases concluídas", "Todas as fases concluídas.", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPlanCompleted(tt.text)
			if result != tt.expected {
				t.Errorf("isPlanCompleted(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestRandomContinuePhrase(t *testing.T) {
	// Verify function returns non-empty string
	for i := 0; i < 50; i++ {
		phrase := randomContinuePhrase()
		if phrase == "" {
			t.Error("randomContinuePhrase() returned empty string")
		}
	}
}

func TestRandomContinuePhrase_Multilingual(t *testing.T) {
	// Verify phrases depend on current language
	tests := []struct {
		lang string
	}{
		{"en"},
		{"ru"},
		{"de"},
		{"es"},
		{"fr"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			i18n.SetLanguage(tt.lang)
			phrase := randomContinuePhrase()
			if phrase == "" {
				t.Errorf("randomContinuePhrase() returned empty for lang=%s", tt.lang)
			}
		})
	}

	// Restore
	i18n.SetLanguage("en")
}

func TestGetCompletionMarkers(t *testing.T) {
	markers := getCompletionMarkers()
	if len(markers) == 0 {
		t.Error("getCompletionMarkers() returned empty list")
	}
	// Verify English markers exist
	foundEn := false
	for _, m := range markers {
		if strings.Contains(m, "all phases") {
			foundEn = true
			break
		}
	}
	if !foundEn {
		t.Error("getCompletionMarkers() should contain English markers")
	}
	// Verify Russian markers exist
	foundRu := false
	for _, m := range markers {
		if strings.Contains(m, "все фазы") {
			foundRu = true
			break
		}
	}
	if !foundRu {
		t.Error("getCompletionMarkers() should contain Russian markers")
	}
}

func TestNewAutoPilotState(t *testing.T) {
	// Default — autoMaxIterations (5000)
	state := NewAutoPilotState(0)
	if state.MaxIterations != autoMaxIterations {
		t.Errorf("NewAutoPilotState(0).MaxIterations = %d, want %d", state.MaxIterations, autoMaxIterations)
	}
	if state.Iteration != 0 {
		t.Errorf("NewAutoPilotState(0).Iteration = %d, want 0", state.Iteration)
	}
	if state.Enabled {
		t.Error("NewAutoPilotState(0).Enabled should be false")
	}

	// Custom limit
	state = NewAutoPilotState(10)
	if state.MaxIterations != 10 {
		t.Errorf("NewAutoPilotState(10).MaxIterations = %d, want 10", state.MaxIterations)
	}

	// Negative — default
	state = NewAutoPilotState(-5)
	if state.MaxIterations != autoMaxIterations {
		t.Errorf("NewAutoPilotState(-5).MaxIterations = %d, want %d", state.MaxIterations, autoMaxIterations)
	}

	// One
	state = NewAutoPilotState(1)
	if state.MaxIterations != 1 {
		t.Errorf("NewAutoPilotState(1).MaxIterations = %d, want 1", state.MaxIterations)
	}
}

func TestNewAutoPilotStateFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.Autopilot.MaxIterations = 100

	// maxIterations=0 → use config value
	state := NewAutoPilotStateFromConfig(cfg, 0)
	if state.MaxIterations != 100 {
		t.Errorf("NewAutoPilotStateFromConfig(cfg, 0).MaxIterations = %d, want 100", state.MaxIterations)
	}

	// maxIterations>0 → use explicit value
	state = NewAutoPilotStateFromConfig(cfg, 25)
	if state.MaxIterations != 25 {
		t.Errorf("NewAutoPilotStateFromConfig(cfg, 25).MaxIterations = %d, want 25", state.MaxIterations)
	}
}

func TestFormatAutoIteration(t *testing.T) {
	i18n.Init("en")

	// Verify formatting
	result := formatAutoIteration(3, 10, "Continue")
	if !strings.Contains(result, "3") || !strings.Contains(result, "10") || !strings.Contains(result, "Continue") {
		t.Errorf("formatAutoIteration(3, 10, 'Continue') = %q, should contain iteration/max/phrase", result)
	}

	// Verify format contains separators
	result = formatAutoIteration(1, 50, "Keep going")
	if !strings.Contains(result, "1") || !strings.Contains(result, "50") {
		t.Errorf("formatAutoIteration(1, 50, 'Keep going') = %q, should contain 1/50", result)
	}
}