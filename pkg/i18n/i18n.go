// Package i18n provides internationalization support for BugBuster Code.
// It loads translations from embedded JSON files and allows switching languages at runtime.
//
// Usage:
//
//	i18n.Init("en")                    // Initialize with English
//	fmt.Println(i18n.T("cli.goodbye")) // "Goodbye! 🐛→💥"
//	i18n.SetLanguage("ru")              // Switch to Russian
//	fmt.Println(i18n.T("cli.goodbye")) // "До встречи! 🐛→💥"
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// current is the active language code (e.g. "en", "ru")
var current = "en"

// translations holds all loaded translations: map[langCode]map[key]value
var translations = make(map[string]map[string]string)

// mu protects translations and current from concurrent access
var mu sync.RWMutex

// initialized tracks whether Init() has been called
var initialized bool

// Init loads embedded translations and optional user overrides, then sets the language.
func Init(lang string) error {
	mu.Lock()
	defer mu.Unlock()

	if !initialized {
		// Load embedded translations from go:embed
		for _, langCode := range availableLanguages {
			data, err := localesFS.ReadFile("locales/" + langCode + ".json")
			if err != nil {
				continue // Skip if file not found
			}
			var m map[string]string
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			translations[langCode] = m
		}

		// Load user overrides from .bugbuster/locales/
		loadUserLocales(".bugbuster/locales")

		initialized = true
	}

	if lang == "" {
		lang = "en"
	}
	current = lang
	return nil
}

// loadUserLocales loads user-provided translation files from a directory.
// User translations override embedded ones with the same keys.
func loadUserLocales(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // Directory doesn't exist, that's fine
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		langCode := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if translations[langCode] == nil {
			translations[langCode] = make(map[string]string)
		}
		for k, v := range m {
			translations[langCode][k] = v
		}
	}
}

// SetLanguage switches the active language at runtime.
func SetLanguage(lang string) {
	mu.Lock()
	current = lang
	mu.Unlock()
}

// Language returns the current language code.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// sprintfFn is a variable holding fmt.Sprintf to prevent go vet from
// treating i18n.T as a format-like function. The format strings come from
// JSON translation files at runtime, so go vet cannot validate them.
var sprintfFn = fmt.Sprintf

// errorfFn is a variable holding fmt.Errorf for error wrapping.
var errorfFn = fmt.Errorf

// T returns the translation for the given key in the current language.
// If the key is not found in the current language, it falls back to English.
// If the key is not found in English either, it returns the key itself.
// Supports fmt.Sprintf-style placeholders via args.
func T(key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()

	// Try current language
	if m, ok := translations[current]; ok {
		if val, ok := m[key]; ok {
			if len(args) > 0 {
				return sprintfFn(val, args...)
			}
			return val
		}
	}

	// Fallback to English
	if current != "en" {
		if m, ok := translations["en"]; ok {
			if val, ok := m[key]; ok {
				if len(args) > 0 {
					return sprintfFn(val, args...)
				}
				return val
			}
		}
	}

	// No translation found — return the key
	if len(args) > 0 {
		return sprintfFn(key, args...)
	}
	return key
}

// E returns a localized error for the given key, wrapping the last argument
// with %w for error chain support. It uses fmt.Errorf internally so that
// errors.Is and errors.As work correctly on the returned error.
// The last %v in the format string is automatically replaced with %w,
// since the last argument is expected to be an error.
// If the key is not found, it falls back to English, then to the key itself.
func E(key string, args ...any) error {
	mu.RLock()
	defer mu.RUnlock()

	// Try current language, then English, then use key as format
	format := ""
	if m, ok := translations[current]; ok {
		if val, ok := m[key]; ok {
			format = val
		}
	}
	if format == "" && current != "en" {
		if m, ok := translations["en"]; ok {
			if val, ok := m[key]; ok {
				format = val
			}
		}
	}
	if format == "" {
		format = key
	}

	// Replace last %v with %w for error wrapping.
	// This ensures the error chain is preserved for errors.Is/As.
	lastV := strings.LastIndex(format, "%v")
	if lastV != -1 {
		format = format[:lastV] + "%w" + format[lastV+2:]
	}

	return errorfFn(format, args...)
}

// AvailableLanguages returns a sorted list of available language codes.
func AvailableLanguages() []string {
	mu.RLock()
	defer mu.RUnlock()

	var langs []string
	for lang := range translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// HasLanguage checks if a language is available.
func HasLanguage(lang string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := translations[lang]
	return ok
}

// LanguageName returns the human-readable name for a language code.
func LanguageName(code string) string {
	names := map[string]string{
		"en": "English",
		"ru": "Русский",
		"es": "Español",
		"fr": "Français",
		"de": "Deutsch",
		"ja": "日本語",
		"zh": "简体中文",
		"pt": "Português",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

// TAll returns translations for the given key from ALL available languages.
// Useful for detection patterns that should match regardless of the current language.
// Returns a slice of non-empty translation values.
func TAll(key string) []string {
	mu.RLock()
	defer mu.RUnlock()

	var results []string
	seen := make(map[string]bool)
	for _, m := range translations {
		if val, ok := m[key]; ok && val != "" && !seen[val] {
			results = append(results, val)
			seen[val] = true
		}
	}
	return results
}
