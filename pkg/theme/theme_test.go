package theme

import (
	"strings"
	"testing"

	"bugbuster-code/pkg/config"
)

func TestParseColorANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType ColorType
		wantANSI int
		wantHex  string
	}{
		{"red", "red", ColorANSI, 31, "#FF5555"},
		{"green", "green", ColorANSI, 32, "#50FA7B"},
		{"yellow", "yellow", ColorANSI, 33, "#F1FA8C"},
		{"blue", "blue", ColorANSI, 34, "#BD93F9"},
		{"cyan", "cyan", ColorANSI, 36, "#8BE9FD"},
		{"magenta", "magenta", ColorANSI, 35, "#FF79C6"},
		{"white", "white", ColorANSI, 37, "#F8F8F2"},
		{"black", "black", ColorANSI, 30, "#000000"},
		{"dim", "dim", ColorANSI, 2, "#6272A4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ParseColor(tt.input)
			if c.Type != tt.wantType {
				t.Errorf("ParseColor(%q).Type = %v, want %v", tt.input, c.Type, tt.wantType)
			}
			if c.ANSI != tt.wantANSI {
				t.Errorf("ParseColor(%q).ANSI = %v, want %v", tt.input, c.ANSI, tt.wantANSI)
			}
			if c.Hex != tt.wantHex {
				t.Errorf("ParseColor(%q).Hex = %v, want %v", tt.input, c.Hex, tt.wantHex)
			}
		})
	}
}

func TestParseColor256(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType ColorType
		wantANSI int
	}{
		{"zero", "0", Color256, 0},
		{"244", "244", Color256, 244},
		{"255", "255", Color256, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ParseColor(tt.input)
			if c.Type != tt.wantType {
				t.Errorf("ParseColor(%q).Type = %v, want %v", tt.input, c.Type, tt.wantType)
			}
			if c.ANSI != tt.wantANSI {
				t.Errorf("ParseColor(%q).ANSI = %v, want %v", tt.input, c.ANSI, tt.wantANSI)
			}
		})
	}
}

func TestParseColorHex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType ColorType
		wantHex  string
	}{
		{"green_hex", "#04B575", ColorHex, "#04B575"},
		{"purple_hex", "#7D56F4", ColorHex, "#7D56F4"},
		{"gray_hex", "#3C3C3C", ColorHex, "#3C3C3C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ParseColor(tt.input)
			if c.Type != tt.wantType {
				t.Errorf("ParseColor(%q).Type = %v, want %v", tt.input, c.Type, tt.wantType)
			}
			if c.Hex != tt.wantHex {
				t.Errorf("ParseColor(%q).Hex = %v, want %v", tt.input, c.Hex, tt.wantHex)
			}
		})
	}
}

func TestParseColorEmpty(t *testing.T) {
	c := ParseColor("")
	if c.Type != ColorANSI {
		t.Errorf("ParseColor(\"\").Type = %v, want ColorANSI", c.Type)
	}
}

func TestANSICode(t *testing.T) {
	tests := []struct {
		name   string
		color  Color
		prefix string
	}{
		{"ansi_red", Color{Type: ColorANSI, ANSI: 31}, "\033[31m"},
		{"256_244", Color{Type: Color256, ANSI: 244}, "\033[38;5;244m"},
		{"hex_green", Color{Type: ColorHex, Hex: "#04B575"}, "\033[38;2;4;181;117m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.color.ANSICode()
			if !strings.HasPrefix(got, "\033[") {
				t.Errorf("ANSICode() = %q, want ANSI escape", got)
			}
			if got != tt.prefix {
				t.Errorf("ANSICode() = %q, want %q", got, tt.prefix)
			}
		})
	}
}

func TestDefaultDarkTheme(t *testing.T) {
	cfg := DefaultDarkTheme()
	if cfg.Mode != "dark" {
		t.Errorf("DefaultDarkTheme().Mode = %q, want \"dark\"", cfg.Mode)
	}
	if cfg.WordWrap != 80 {
		t.Errorf("DefaultDarkTheme().WordWrap = %d, want 80", cfg.WordWrap)
	}
	if cfg.Colors.Primary != "cyan" {
		t.Errorf("DefaultDarkTheme().Colors.Primary = %q, want \"cyan\"", cfg.Colors.Primary)
	}
	if cfg.Colors.UserMsg != "#04B575" {
		t.Errorf("DefaultDarkTheme().Colors.UserMsg = %q, want \"#04B575\"", cfg.Colors.UserMsg)
	}
}

func TestDefaultLightTheme(t *testing.T) {
	cfg := DefaultLightTheme()
	if cfg.Mode != "light" {
		t.Errorf("DefaultLightTheme().Mode = %q, want \"light\"", cfg.Mode)
	}
}

func TestResolveTheme(t *testing.T) {
	cfg := config.ThemeConfig{
		Mode:     "dark",
		WordWrap: 80,
	}
	rt := ResolveTheme(cfg)

	if rt.Mode != "dark" {
		t.Errorf("ResolveTheme().Mode = %q, want \"dark\"", rt.Mode)
	}
	if rt.WordWrap != 80 {
		t.Errorf("ResolveTheme().WordWrap = %d, want 80", rt.WordWrap)
	}
	// Проверяем что Primary = cyan (из дефолтной тёмной темы)
	if rt.Primary.Value != "cyan" {
		t.Errorf("ResolveTheme().Primary.Value = %q, want \"cyan\"", rt.Primary.Value)
	}
	if rt.Primary.ANSI != 36 {
		t.Errorf("ResolveTheme().Primary.ANSI = %d, want 36", rt.Primary.ANSI)
	}
	// Проверяем hex-цвет
	if rt.UserMsg.Hex != "#04B575" {
		t.Errorf("ResolveTheme().UserMsg.Hex = %q, want \"#04B575\"", rt.UserMsg.Hex)
	}
}

func TestResolveThemeOverride(t *testing.T) {
	cfg := config.ThemeConfig{
		Mode:     "dark",
		WordWrap: 120,
		Colors: config.ThemeColors{
			Primary: "blue", // перекрываем дефолтный cyan
		},
	}
	rt := ResolveTheme(cfg)

	if rt.WordWrap != 120 {
		t.Errorf("ResolveTheme().WordWrap = %d, want 120", rt.WordWrap)
	}
	if rt.Primary.Value != "blue" {
		t.Errorf("ResolveTheme().Primary.Value = %q, want \"blue\"", rt.Primary.Value)
	}
	// Success не перекрыт — должен быть из дефолтной темы
	if rt.Success.Value != "green" {
		t.Errorf("ResolveTheme().Success.Value = %q, want \"green\" (default)", rt.Success.Value)
	}
}

func TestColor256ToHex(t *testing.T) {
	tests := []struct {
		input   int
		wantHex string
	}{
		{0, "#000000"},
		{15, "#FFFFFF"},
		{16, "#000000"},  // начало 216-палитты
		{244, "#808080"}, // оттенок серого (приближение)
		{255, "#EEEEEE"},
	}

	for _, tt := range tests {
		got := color256ToHex(tt.input)
		if got != tt.wantHex {
			t.Errorf("color256ToHex(%d) = %q, want %q", tt.input, got, tt.wantHex)
		}
	}
}

func TestHexToRGB(t *testing.T) {
	r, g, b := hexToRGB("#04B575")
	if r != 4 || g != 181 || b != 117 {
		t.Errorf("hexToRGB(\"#04B575\") = (%d, %d, %d), want (4, 181, 117)", r, g, b)
	}
}

func TestLipglossColor(t *testing.T) {
	c := ParseColor("#04B575")
	lc := c.LipglossColor()
	if lc == nil {
		t.Error("LipglossColor() returned nil")
	}
}
