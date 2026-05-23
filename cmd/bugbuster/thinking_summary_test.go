package main

import (
	"testing"
)

func TestIsCodeLikeLine(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		// Кодоподобные строки — должны быть true
		{"};", true},
		{");", true},
		{"}", true},
		{"pub use uni::{Uni, UniValue};", true},
		{"fn main() {", true},
		{"let x = 5;", true},
		{"import os", true},
		{"self.value = 42;", true},
		{"std::collections::HashMap", true},
		{"Vec<String>", true},
		{"Option<Result<String, Error>>", true},
		{"return Ok(());", true},
		{"impl Display for Foo {", true},
		{"mod tests;", true},
		{"type Result<T> = std::result::Result<T, Error>;", true},
		{"const MAX_SIZE: usize = 1024;", true},
		{"if let Some(x) = value {", true},
		{"for item in items.iter() {", true},
		{"match result {", true},
		{"case 'a':", true},
		{"// comment", true},
		{"/* block */", true},
		{"#[derive(Debug)]", true},
		{"self.data.push(value);", true},
		{"crate::module::function()", true},
		{"String::from(\"hello\")", true},
		{"fmt::Display", true},
		{"io::Error", true},
		{"class MyClass:", true},
		{"def method(self):", true},
		{"var x = 10;", true},
		{"super.method()", true},

		// Нормальные строки мышления — должны быть false
		{"Мне нужно проанализировать структуру проекта", false},
		{"Давайте посмотрим на файлы", false},
		{"The user wants me to fix the bug", false},
		{"I'll check the configuration first", false},
		{"Сначала прочитаю файл конфигурации", false},
		{"Looking at the error message", false},
		{"Now I need to understand the flow", false},
		{"Это выглядит как проблема с импортами", false},
		{"Проверю зависимости", false},
		{"Let me examine the code structure", false}, // "let me" — английская фраза, не код
		{"Анализирую ошибку в функции обработки", false},

		// Короткие неинформативные строки — true
		{"", true},
		{"  ", true},
		{"a", true},
		{"ab", true},
		{"abc", true},
		{"OK", true},  // слишком короткое (2 руны)
		{"Да", true},   // слишком короткое (2 руны)

		// Пограничные случаи — нормальные строки длиной > 3 рун
		{"Хорошо, давайте проверим", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := isCodeLikeLine(tt.line)
			if result != tt.expected {
				t.Errorf("isCodeLikeLine(%q) = %v, expected %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestSummarizeThinking(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "normal thinking text",
			input:    "Мне нужно проанализировать код\nи найти ошибку в логике",
			expected: "и найти ошибку в логике",
		},
		{
			name:     "code at end — skip to previous meaningful line",
			input:    "Давайте проверим файл\npub use uni::{Uni, UniValue};",
			expected: "Давайте проверим файл",
		},
		{
			name:     "closing brace at end",
			input:    "Анализирую структуру\n};",
			expected: "Анализирую структуру",
		},
		{
			name:     "all code lines — return empty",
			input:    "fn main() {\nlet x = 5;\n};",
			expected: "",
		},
		{
			name:     "long line — truncated",
			input:    "Это очень длинная строка которая превышает восемьдесят символов и должна быть обрезана в конце чтобы уместиться",
			expected: "Это очень длинная строка которая превышае...",  // 77 + "..."
		},
		{
			name:     "mixed code and text",
			input:    "Сначала проверю конфигурацию\nimport os\nПотом посмотрю логи",
			expected: "Потом посмотрю логи",
		},
		{
			name:     "single meaningful line",
			input:    "Нужно исправить баг в обработчике",
			expected: "Нужно исправить баг в обработчике",
		},
		{
			name:     "trailing empty lines",
			input:    "Проверяю файл\n\n\n",
			expected: "Проверяю файл",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeThinking(tt.input)
			if result != tt.expected {
				t.Errorf("summarizeThinking(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}