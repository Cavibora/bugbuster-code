package main

import "testing"

func TestIsYesAnswer(t *testing.T) {
	tests := []struct {
		answer string
		want   bool
	}{
		// English
		{"y", true},
		{"yes", true},
		{"Y", true},
		{"YES", true},
		// Russian
		{"д", true},
		{"да", true},
		{"Д", true},
		{"ДА", true},
		// German
		{"j", true},
		{"ja", true},
		{"JA", true},
		// Spanish
		{"s", true},
		{"sí", true},
		// French
		{"o", true},
		{"oui", true},
		// Portuguese
		{"sim", true},
		// Japanese
		{"はい", true},
		{"hai", true},
		// Chinese
		{"是", true},
		{"shi", true},
		// Negative
		{"n", false},
		{"no", false},
		{"нет", false},
		{"nein", false},
		{"", false},
		{"maybe", false},
	}

	for _, tt := range tests {
		// isYesAnswer expects lowercase input
		got := isYesAnswer(tt.answer)
		if got != tt.want {
			t.Errorf("isYesAnswer(%q) = %v, want %v", tt.answer, got, tt.want)
		}
	}
}
