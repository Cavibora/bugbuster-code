package i18n

import "embed"

//go:embed locales/*.json
var localesFS embed.FS

// availableLanguages lists all embedded language codes.
// Must match the .json files in locales/ directory.
var availableLanguages = []string{
	"en", // English (default)
	"ru", // Russian
	"es", // Español
	"fr", // Français
	"de", // Deutsch
	"ja", // 日本語
	"zh", // 简体中文
	"pt", // Português
}
