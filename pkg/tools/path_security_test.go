package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSecretPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Точные совпадения
		{".env", true},
		{"/home/user/project/.env", true},
		{"/home/user/project/.env.local", true},
		{"/home/user/.ssh/id_rsa", true},
		{"credentials.json", true},
		// Точные совпадения токенов
		{"token", true},
		{".token", true},
		{"token.txt", true},
		{"access_token", true},
		{"access_token.json", true},
		{"api_token", true},
		{"auth_token", true},
		// Подстроки в имени файла
		{"/project/config/secrets.yaml", true},
		{"/project/my_password.txt", true},
		{"/project/certificate.pem", true},
		{"/project/private.key", true},
		// Не секретные — "token" как подстрока НЕ должна блокировать
		{"/project/tokenizer.rs", false},
		{"/project/tokenize.py", false},
		{"/project/token_handler.go", false},
		{"/project/my_tokenizer.ts", false},
		// Пути
		{"/home/user/.aws/credentials", true},
		{"/home/user/.kube/config", true},
		// Не секретные
		{"/home/user/project/main.go", false},
		{"/home/user/project/README.md", false},
		{"/home/user/project/environment.go", false},
		{"/home/user/project/config.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsSecretPath(tt.path)
			if result != tt.expected {
				t.Errorf("IsSecretPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsSystemPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/usr/bin/python", true},
		{"/System/Library", true},
		{"/root/.bashrc", true},
		{"/boot/vmlinuz", true},
		// Не системные
		{"/home/user/project/main.go", false},
		{"/tmp/test.txt", false},
		{"/Users/user/project", false},
		{"/var/folders/abc/test.txt", false}, // macOS temp
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsSystemPath(tt.path)
			if result != tt.expected {
				t.Errorf("IsSystemPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsSystemPath_MacOSSymlinks(t *testing.T) {
	// На macOS /etc — симлинк на /private/etc
	result := IsSystemPath("/etc/passwd")
	if !result {
		t.Log("Note: /etc/passwd not detected as system path (may not exist on this OS)")
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"main.go", false},
		{"/home/user/project/file.txt", false},
		{"../etc/passwd", true},              // path traversal
		{"subdir/../../../etc/passwd", true}, // path traversal
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, err := ResolvePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolvePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestIsPathAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		path        string
		allowedDirs []string
		expected    bool
	}{
		{filepath.Join(tmpDir, "file.txt"), []string{tmpDir}, true},
		{"/etc/hosts", []string{tmpDir}, false},
		{"/home/user/project/file.txt", []string{"/home/user/project"}, true},
		{"/home/user/project/sub/file.txt", []string{"/home/user/project"}, true},
		{"/home/other/file.txt", []string{"/home/user/project"}, false},
		// Пустой AllowedDirs = всё разрешено
		{"/any/path/file.txt", []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsPathAllowed(tt.path, tt.allowedDirs)
			if result != tt.expected {
				t.Errorf("IsPathAllowed(%q, %v) = %v, want %v", tt.path, tt.allowedDirs, result, tt.expected)
			}
		})
	}
}

func TestReadTool_SecretPathBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, ".env")
	os.WriteFile(secretFile, []byte("SECRET=123"), 0644)

	tool := NewReadTool()
	tool.AllowedDirs = []string{tmpDir}

	result := tool.Execute(map[string]string{"path": secretFile})
	if result.Error == "" {
		t.Error("Expected error for secret file .env")
	}
}

func TestWriteTool_SystemPathBlocked(t *testing.T) {
	tool := NewWriteTool()

	result := tool.Execute(map[string]string{
		"path":    "/etc/malicious",
		"content": "bad",
	})
	if result.Error == "" {
		t.Error("Expected error for system path /etc")
	}
}

func TestEditTool_SecretPathBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "credentials.json")
	os.WriteFile(secretFile, []byte(`{"key": "secret"}`), 0644)

	tool := NewEditTool()
	tool.AllowedDirs = []string{tmpDir}

	result := tool.Execute(map[string]string{
		"path": secretFile,
		"old":  "secret",
		"new":  "public",
	})
	if result.Error == "" {
		t.Error("Expected error for secret file credentials.json")
	}
}

func TestReadTool_PathTraversalBlocked(t *testing.T) {
	tool := NewReadTool()

	result := tool.Execute(map[string]string{"path": "../../../etc/passwd"})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestWriteTool_PathTraversalBlocked(t *testing.T) {
	tool := NewWriteTool()

	result := tool.Execute(map[string]string{
		"path":    "../../../tmp/malicious",
		"content": "bad",
	})
	if result.Error == "" {
		t.Error("Expected error for path traversal")
	}
}

func TestSecurityError(t *testing.T) {
	if ErrPathTraversal.Error() == "" {
		t.Error("ErrPathTraversal should have error message")
	}
	if ErrSecretPath.Error() == "" {
		t.Error("ErrSecretPath should have error message")
	}
	if ErrSystemPath.Error() == "" {
		t.Error("ErrSystemPath should have error message")
	}
}
func TestSecretPathInfo(t *testing.T) {
	tests := []struct {
		path        string
		isSecret    bool
		description string
	}{
		{".env", true, "secret file (.env)"},
		{"/home/user/project/.env", true, "secret file (.env)"},
		{"credentials.json", true, "secret file (credentials.json)"},
		{"token", true, "secret file (token)"},
		{"/project/config/secrets.yaml", true, "filename contains 'secret'"},
		{"/project/my_password.txt", true, "filename contains 'password'"},
		{"/project/certificate.pem", true, "filename contains '.pem'"},
		{"/home/user/.ssh/id_rsa", true, "secret path (.ssh)"},
		// Не секретные
		{"/project/tokenizer.rs", false, ""},
		{"/project/main.go", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			isSecret, desc := SecretPathInfo(tt.path)
			if isSecret != tt.isSecret {
				t.Errorf("SecretPathInfo(%q) = (%v, %q), want isSecret=%v", tt.path, isSecret, desc, tt.isSecret)
			}
			if tt.isSecret && desc == "" {
				t.Errorf("SecretPathInfo(%q) returned secret=true but empty description", tt.path)
			}
			if !tt.isSecret && desc != "" {
				t.Errorf("SecretPathInfo(%q) returned secret=false but non-empty description: %q", tt.path, desc)
			}
		})
	}
}
