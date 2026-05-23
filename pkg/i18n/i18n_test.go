package i18n

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInit_DefaultLanguage(t *testing.T) {
	err := Init("en")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if lang := Language(); lang != "en" {
		t.Errorf("Expected language 'en', got '%s'", lang)
	}
}

func TestInit_RussianLanguage(t *testing.T) {
	err := Init("ru")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if lang := Language(); lang != "ru" {
		t.Errorf("Expected language 'ru', got '%s'", lang)
	}
}

func TestInit_EmptyLanguage(t *testing.T) {
	err := Init("")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if lang := Language(); lang != "en" {
		t.Errorf("Expected default language 'en', got '%s'", lang)
	}
}

func TestT_English(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test simple key
	val := T("cli.goodbye")
	if val != "Goodbye! 🐛→💥" {
		t.Errorf("Expected 'Goodbye! 🐛→💥', got '%s'", val)
	}
}

func TestT_Russian(t *testing.T) {
	if err := Init("ru"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	val := T("cli.goodbye")
	if val != "До встречи! 🐛→💥" {
		t.Errorf("Expected 'До встречи! 🐛→💥', got '%s'", val)
	}
}

func TestT_FallbackToEnglish(t *testing.T) {
	// Set language to one that has partial translations
	if err := Init("de"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Key exists in German
	val := T("cli.goodbye")
	if val != "Auf Wiedersehen! 🐛→💥" {
		t.Errorf("Expected German translation, got '%s'", val)
	}

	// Key missing in German, should fall back to English
	val = T("cli_error.provider_create")
	if val == "cli_error.provider_create" {
		t.Errorf("Expected English fallback, got key itself")
	}
}

func TestT_FallbackToKey(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Key doesn't exist in any language
	val := T("nonexistent.key.xyz")
	if val != "nonexistent.key.xyz" {
		t.Errorf("Expected key itself as fallback, got '%s'", val)
	}
}

func TestT_WithArgs(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Use a key that has %s in its translation value
	key := "cli.unknown_command"
	val := T(key, "test")
	if val != "Unknown command: test. Type /help for help." {
		t.Errorf("Expected formatted string, got '%s'", val)
	}
}

func TestT_WithArgsRussian(t *testing.T) {
	if err := Init("ru"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Use a key that has %s in its translation value
	key := "cli.unknown_command"
	val := T(key, "test")
	if val != "Неизвестная команда: test. Введите /help для справки." {
		t.Errorf("Expected formatted Russian string, got '%s'", val)
	}
}

func TestT_WithArgsFallbackToKey(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Key doesn't exist, args should be applied to key itself
	val := T("nonexistent.%s.key", "test")
	if val != "nonexistent.test.key" {
		t.Errorf("Expected key with args applied, got '%s'", val)
	}
}

func TestSetLanguage(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	SetLanguage("ru")
	if lang := Language(); lang != "ru" {
		t.Errorf("Expected 'ru', got '%s'", lang)
	}

	// Verify translation changes
	val := T("cli.goodbye")
	if val != "До встречи! 🐛→💥" {
		t.Errorf("Expected Russian translation after SetLanguage, got '%s'", val)
	}

	SetLanguage("en")
	val = T("cli.goodbye")
	if val != "Goodbye! 🐛→💥" {
		t.Errorf("Expected English translation after SetLanguage, got '%s'", val)
	}
}

func TestAvailableLanguages(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	langs := AvailableLanguages()
	if len(langs) == 0 {
		t.Error("Expected non-empty languages list")
	}

	// Check that en and ru are present
	foundEn := false
	foundRu := false
	for _, lang := range langs {
		if lang == "en" {
			foundEn = true
		}
		if lang == "ru" {
			foundRu = true
		}
	}
	if !foundEn {
		t.Error("Expected 'en' in available languages")
	}
	if !foundRu {
		t.Error("Expected 'ru' in available languages")
	}
}

func TestHasLanguage(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if !HasLanguage("en") {
		t.Error("Expected 'en' to be available")
	}
	if !HasLanguage("ru") {
		t.Error("Expected 'ru' to be available")
	}
	if HasLanguage("xx") {
		t.Error("Expected 'xx' to not be available")
	}
}

func TestLanguageName(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"en", "English"},
		{"ru", "Русский"},
		{"es", "Español"},
		{"fr", "Français"},
		{"de", "Deutsch"},
		{"ja", "日本語"},
		{"zh", "简体中文"},
		{"pt", "Português"},
		{"xx", "xx"}, // Unknown code returns itself
	}

	for _, tt := range tests {
		name := LanguageName(tt.code)
		if name != tt.expected {
			t.Errorf("LanguageName(%s) = '%s', expected '%s'", tt.code, name, tt.expected)
		}
	}
}

func TestUserLocales(t *testing.T) {
	// Create a temporary locale directory
	tmpDir := t.TempDir()
	localeDir := filepath.Join(tmpDir, ".bugbuster", "locales")
	if err := os.MkdirAll(localeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a custom translation file
	customTranslations := map[string]string{
		"cli.goodbye": "Custom goodbye!",
		"custom.key":  "Custom value",
	}
	data, _ := json.Marshal(customTranslations)
	if err := os.WriteFile(filepath.Join(localeDir, "en.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Reset initialized state
	initialized = false
	translations = make(map[string]map[string]string)

	// Init should load user overrides
	err := Init("en")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Note: user locales are loaded from ".bugbuster/locales" relative to CWD,
	// not from our tmpDir, so this test verifies the mechanism exists
	// but doesn't test the actual override in this test environment
}

func TestAllLocaleFilesValid(t *testing.T) {
	// Verify all embedded locale files are valid JSON
	for _, langCode := range availableLanguages {
		data, err := localesFS.ReadFile("locales/" + langCode + ".json")
		if err != nil {
			t.Errorf("Failed to read locale file for '%s': %v", langCode, err)
			continue
		}

		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Errorf("Invalid JSON in locale file '%s': %v", langCode, err)
		}

		if len(m) == 0 {
			t.Errorf("Locale file '%s' is empty", langCode)
		}
	}
}

func TestEnglishHasAllKeys(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// English should have the most keys
	enTranslations := translations["en"]
	if len(enTranslations) < 100 {
		t.Errorf("English translations should have at least 100 keys, got %d", len(enTranslations))
	}
}

func TestRussianHasMostKeys(t *testing.T) {
	if err := Init("ru"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ruTranslations := translations["ru"]
	enTranslations := translations["en"]

	// Russian should have most of the English keys
	if len(ruTranslations) < len(enTranslations)*80/100 {
		t.Errorf("Russian should have at least 80%% of English keys, got %d/%d",
			len(ruTranslations), len(enTranslations))
	}
}

func TestConcurrentAccess(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test that T() is safe for concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				T("cli.goodbye")
				Language()
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestNewCLIKeys(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test new keys added for main.go i18n
	newKeys := []string{
		"cli.short_desc",
		"cli.long_desc",
		"cli.version_info",
		"cli.version_subtitle",
		"cli.subcommands_header",
		"cli.tool_call_start",
		"cli.tool_call_end",
		"cli_success.config_init_hint",
	}

	for _, key := range newKeys {
		val := T(key)
		if val == key {
			t.Errorf("Key '%s' not found in English translations", key)
		}
		if val == "" {
			t.Errorf("Key '%s' has empty value", key)
		}
	}
}

func TestE_ErrorWrapping(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test that E wraps errors with %w
	innerErr := os.ErrNotExist
	err := E("errors_config.read", innerErr)

	if err == nil {
		t.Fatal("Expected non-nil error")
	}

	// Check that the error message contains the inner error text
	if !contains(err.Error(), "file does not exist") && !contains(err.Error(), "ErrNotExist") {
		t.Errorf("Expected error to contain inner error text, got '%s'", err.Error())
	}

	// Check that errors.Is works (this is the key benefit of %w wrapping)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Expected errors.Is to find os.ErrNotExist, got error: %v", err)
	}
}

func TestE_WithMultipleArgs(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test E with format string that has %s and %v (last one becomes %w)
	innerErr := os.ErrPermission
	err := E("errors_provider.request", "OpenAI", innerErr)

	if err == nil {
		t.Fatal("Expected non-nil error")
	}

	// Check that the error message contains the provider name
	if !contains(err.Error(), "OpenAI") {
		t.Errorf("Expected error to contain 'OpenAI', got '%s'", err.Error())
	}

	// Check that errors.Is works
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("Expected errors.Is to find os.ErrPermission, got error: %v", err)
	}
}

func TestE_FallbackToEnglish(t *testing.T) {
	// Set language to one with partial translations
	if err := Init("de"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	innerErr := os.ErrNotExist
	err := E("errors_config.read", innerErr)

	// Should fall back to English format string
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Expected errors.Is to find os.ErrNotExist, got error: %v", err)
	}
}

func TestE_FallbackToKey(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	innerErr := os.ErrNotExist
	err := E("nonexistent.error.key.%v", innerErr)

	// Should use key as format string, with %v replaced by %w
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Expected errors.Is to find os.ErrNotExist, got error: %v", err)
	}
}

func TestE_NoPercentV(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test E with a key that has no %v in format string
	err := E("cli.goodbye")
	if err == nil {
		t.Fatal("Expected non-nil error")
	}
	// Should still work, just no wrapping
	if err.Error() != "Goodbye! 🐛→💥" {
		t.Errorf("Expected 'Goodbye! 🐛→💥', got '%s'", err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" ||
		(s != "" && substr != "" && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNewCLIKeysRussian(t *testing.T) {
	if err := Init("ru"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Test new keys in Russian
	newKeys := []string{
		"cli.short_desc",
		"cli.long_desc",
		"cli.version_info",
		"cli.version_subtitle",
		"cli.subcommands_header",
		"cli.tool_call_start",
		"cli.tool_call_end",
		"cli_success.config_init_hint",
	}

	for _, key := range newKeys {
		val := T(key)
		if val == key {
			t.Errorf("Key '%s' not found in Russian translations", key)
		}
	}
}

func TestTAll(t *testing.T) {
	// Init is required to load translations
	if err := Init("en"); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	results := TAll("cli.goodbye")
	if len(results) == 0 {
		t.Error("Expected at least one translation for cli.goodbye")
	}
	seen := make(map[string]bool)
	for _, val := range results {
		if seen[val] {
			t.Errorf("Duplicate value in TAll results: %s", val)
		}
		seen[val] = true
	}
}

func TestTAll_NonExistentKey(t *testing.T) {
	results := TAll("nonexistent.key.xyz")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for nonexistent key, got %d", len(results))
	}
}
