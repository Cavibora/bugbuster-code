package theme

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bugbuster-code/pkg/config"
)

// ColorType is color type
type ColorType int

const (
	ColorANSI ColorType = iota // red, green, cyan...
	Color256                   // "244"
	ColorHex                   // "#04B575"
)

// Color is a parsed color with precomputed ANSI and lipgloss values
type Color struct {
	Type  ColorType
	Value string // original line
	ANSI  int    // ANSI code (0-255) for generating escape code
	Hex   string // "#RRGGBB" for lipgloss
}

// ansiNameMap — ANSI name to code mapping
var ansiNameMap = map[string]int{
	"black":   30,
	"red":     31,
	"green":   32,
	"yellow":  33,
	"blue":    34,
	"magenta": 35,
	"cyan":    36,
	"white":   37,
	"dim":     2,
}

// ansiNameToHex — ANSI name to hex mapping (for lipgloss)
var ansiNameToHex = map[string]string{
	"black":   "#000000",
	"red":     "#FF5555",
	"green":   "#50FA7B",
	"yellow":  "#F1FA8C",
	"blue":    "#BD93F9",
	"magenta": "#FF79C6",
	"cyan":    "#8BE9FD",
	"white":   "#F8F8F2",
	"dim":     "#6272A4",
}

// ParseColor parses color line into Color.
// Supports: ANSI names (red, green, cyan...), 256-color numbers ("244"), hex ("#04B575")
func ParseColor(s string) Color {
	if s == "" {
		return Color{Type: ColorANSI, Value: "", ANSI: 0, Hex: "#000000"}
	}

	s = strings.ToLower(strings.TrimSpace(s))

	// Check ANSI name
	if code, ok := ansiNameMap[s]; ok {
		hex, _ := ansiNameToHex[s]
		return Color{Type: ColorANSI, Value: s, ANSI: code, Hex: hex}
	}

	// Check hex (#RRGGBB)
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		return Color{Type: ColorHex, Value: s, ANSI: 0, Hex: strings.ToUpper(s)}
	}

	// Check 256-color number
	if num, err := strconv.Atoi(s); err == nil && num >= 0 && num <= 255 {
		hex := color256ToHex(num)
		return Color{Type: Color256, Value: s, ANSI: num, Hex: hex}
	}

	// Fallback — try as ANSI name
	return Color{Type: ColorANSI, Value: s, ANSI: 37, Hex: "#F8F8F2"}
}

// ANSICode returns ANSI escape code for color
func (c Color) ANSICode() string {
	switch c.Type {
	case ColorANSI:
		return fmt.Sprintf("\033[%dm", c.ANSI)
	case Color256:
		return fmt.Sprintf("\033[38;5;%dm", c.ANSI)
	case ColorHex:
		r, g, b := hexToRGB(c.Hex)
		return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
	default:
		return "\033[37m" // white fallback
	}
}

// LipglossColor returns color.Color for lipgloss v2
func (c Color) LipglossColor() color.Color {
	return lipgloss.Color(c.Hex)
}

// color256ToHex — converts 256-color number to hex (approximation)
func color256ToHex(n int) string {
	// Standard 16 colors
	if n < 16 {
		standardHex := [16]string{
			"#000000", "#800000", "#008000", "#808000",
			"#000080", "#800080", "#008080", "#C0C0C0",
			"#808080", "#FF0000", "#00FF00", "#FFFF00",
			"#0000FF", "#FF00FF", "#00FFFF", "#FFFFFF",
		}
		return standardHex[n]
	}
	// Color palette 216 (16-231)
	if n >= 16 && n <= 231 {
		n -= 16
		r := (n / 36) * 51
		g := ((n % 36) / 6) * 51
		b := (n % 6) * 51
		if r > 255 {
			r = 255
		}
		if g > 255 {
			g = 255
		}
		if b > 255 {
			b = 255
		}
		return fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	// Gray shades (232-255)
	if n >= 232 && n <= 255 {
		v := 8 + (n-232)*10
		if v > 255 {
			v = 255
		}
		return fmt.Sprintf("#%02X%02X%02X", v, v, v)
	}
	return "#000000"
}

// hexToRGB — parses "#RRGGBB" into r, g, b
func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 32)
	g, _ := strconv.ParseInt(hex[2:4], 16, 32)
	b, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return int(r), int(g), int(b)
}

// ResolvedTheme — resolved theme with precomputed colors
type ResolvedTheme struct {
	Mode     string
	WordWrap int
	Primary  *Color
	Success  *Color
	Error    *Color
	Warning  *Color
	Info     *Color
	Dim      *Color

	Thinking    *Color
	ToolParams  *Color
	ToolSummary *Color
	StatusTime  *Color
	StatusSep   *Color
	CtxGood     *Color
	CtxWarn     *Color
	CtxBad      *Color
	UserMsg     *Color
	Assistant   *Color
	Separator   *Color
}

// DefaultDarkTheme returns default dark theme (matches current hardcodes)
func DefaultDarkTheme() config.ThemeConfig {
	return config.ThemeConfig{
		Mode:     "dark",
		WordWrap: 80,
		Colors: config.ThemeColors{
			Primary:     "cyan",
			Success:     "green",
			Error:       "red",
			Warning:     "yellow",
			Info:        "blue",
			Dim:         "244",
			Thinking:    "244",
			ToolParams:  "cyan",
			ToolSummary: "244",
			StatusTime:  "244",
			StatusSep:   "244",
			CtxGood:     "green",
			CtxWarn:     "yellow",
			CtxBad:      "red",
			UserMsg:     "#04B575",
			Assistant:   "#7D56F4",
			Separator:   "#3C3C3C",
		},
	}
}

// DefaultLightTheme returns default light theme
func DefaultLightTheme() config.ThemeConfig {
	return config.ThemeConfig{
		Mode:     "light",
		WordWrap: 80,
		Colors: config.ThemeColors{
			Primary:     "blue",
			Success:     "green",
			Error:       "red",
			Warning:     "yellow",
			Info:        "blue",
			Dim:         "246",
			Thinking:    "246",
			ToolParams:  "blue",
			ToolSummary: "246",
			StatusTime:  "246",
			StatusSep:   "246",
			CtxGood:     "green",
			CtxWarn:     "yellow",
			CtxBad:      "red",
			UserMsg:     "#006400",
			Assistant:   "#6A0DAD",
			Separator:   "#CCCCCC",
		},
	}
}

// ResolveTheme resolves ThemeConfig into ResolvedTheme with precomputed colors
func ResolveTheme(cfg config.ThemeConfig) *ResolvedTheme {
	// Determine default theme based on mode
	var defaults config.ThemeColors
	if cfg.Mode == "light" {
		defaults = DefaultLightTheme().Colors
	} else {
		defaults = DefaultDarkTheme().Colors
	}

	// Merge: non-empty fields from cfg override defaults
	merged := mergeColors(defaults, cfg.Colors)

	return &ResolvedTheme{
		Mode:     cfg.Mode,
		WordWrap: cfg.WordWrap,
		Primary:  ptrColor(ParseColor(merged.Primary)),
		Success:  ptrColor(ParseColor(merged.Success)),
		Error:    ptrColor(ParseColor(merged.Error)),
		Warning:  ptrColor(ParseColor(merged.Warning)),
		Info:     ptrColor(ParseColor(merged.Info)),
		Dim:      ptrColor(ParseColor(merged.Dim)),

		Thinking:    ptrColor(ParseColor(merged.Thinking)),
		ToolParams:  ptrColor(ParseColor(merged.ToolParams)),
		ToolSummary: ptrColor(ParseColor(merged.ToolSummary)),
		StatusTime:  ptrColor(ParseColor(merged.StatusTime)),
		StatusSep:   ptrColor(ParseColor(merged.StatusSep)),
		CtxGood:     ptrColor(ParseColor(merged.CtxGood)),
		CtxWarn:     ptrColor(ParseColor(merged.CtxWarn)),
		CtxBad:      ptrColor(ParseColor(merged.CtxBad)),
		UserMsg:     ptrColor(ParseColor(merged.UserMsg)),
		Assistant:   ptrColor(ParseColor(merged.Assistant)),
		Separator:   ptrColor(ParseColor(merged.Separator)),
	}
}

// mergeColors merges colors: non-empty fields from overrides override defaults
func mergeColors(defaults, overrides config.ThemeColors) config.ThemeColors {
	result := defaults
	if overrides.Primary != "" {
		result.Primary = overrides.Primary
	}
	if overrides.Success != "" {
		result.Success = overrides.Success
	}
	if overrides.Error != "" {
		result.Error = overrides.Error
	}
	if overrides.Warning != "" {
		result.Warning = overrides.Warning
	}
	if overrides.Info != "" {
		result.Info = overrides.Info
	}
	if overrides.Dim != "" {
		result.Dim = overrides.Dim
	}
	if overrides.Thinking != "" {
		result.Thinking = overrides.Thinking
	}
	if overrides.ToolParams != "" {
		result.ToolParams = overrides.ToolParams
	}
	if overrides.ToolSummary != "" {
		result.ToolSummary = overrides.ToolSummary
	}
	if overrides.StatusTime != "" {
		result.StatusTime = overrides.StatusTime
	}
	if overrides.StatusSep != "" {
		result.StatusSep = overrides.StatusSep
	}
	if overrides.CtxGood != "" {
		result.CtxGood = overrides.CtxGood
	}
	if overrides.CtxWarn != "" {
		result.CtxWarn = overrides.CtxWarn
	}
	if overrides.CtxBad != "" {
		result.CtxBad = overrides.CtxBad
	}
	if overrides.UserMsg != "" {
		result.UserMsg = overrides.UserMsg
	}
	if overrides.Assistant != "" {
		result.Assistant = overrides.Assistant
	}
	if overrides.Separator != "" {
		result.Separator = overrides.Separator
	}
	return result
}

func ptrColor(c Color) *Color {
	return &c
}
