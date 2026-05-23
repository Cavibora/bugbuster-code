package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Secret files and patterns — block read/write
// Exact matches of file/directory names
var secretExactNames = []string{
	".env",
	".env.local",
	".env.production",
	".env.development",
	"credentials.json",
	"credentials",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	".htpasswd",
	".netrc",
	".npmrc",
	".pypirc",
	"token",     // file without extension named "token"
	".token",    // hidden file .token
	"token.txt", // file with tokens
	"access_token",
	"access_token.json",
	"refresh_token",
	"api_token",
	"auth_token",
}

// Patterns for substring match in filename
// Note: "token" is removed from substrings as it falsely blocks
// files like tokenizer.rs, tokenize.py, token_handler.go.
// Instead, use secretExactNames for specific token files.
var secretSubstrings = []string{
	"secret",
	"password",
	".pem",
	".key",
}

// Secret paths (directories and files by paths)
var secretPathPatterns = []string{
	".ssh",
	".aws/credentials",
	".aws/config",
	".kube/config",
	"config/gcloud",
}

// System paths — block write
var systemPaths = []string{
	"/etc",
	"/private/etc",
	"/usr",
	"/System",
	"/Library/System",
	"/sbin",
	"/bin",
	"/boot",
	"/dev",
	"/proc",
	"/sys",
	"/var/log",
	"/var/run",
	"/var/lib",
	"/var/spool",
	"/private/var/log",
	"/private/var/run",
	"/private/var/lib",
	"/private/var/spool",
	"/root",
	"/Windows",
	"/Program Files",
	"/ProgramData",
}

// IsSecretPath checks if path contains secret files
func IsSecretPath(path string) bool {
	isSecret, _ := SecretPathInfo(path)
	return isSecret
}

// SecretPathInfo checks if path contains secret files,
// and returns a description of the blocking reason.
// If path is secret — returns (true, description).
// If not — (false, "").
func SecretPathInfo(path string) (bool, string) {
	cleanPath := filepath.Clean(path)
	base := filepath.Base(cleanPath)
	lowerBase := strings.ToLower(base)
	lowerPath := strings.ToLower(cleanPath)

	// 1. Exact filename match
	for _, name := range secretExactNames {
		if lowerBase == strings.ToLower(name) {
			return true, fmt.Sprintf("secret file (%s)", base)
		}
	}

	// 2. Substring match in filename (secret, password, .pem, .key)
	for _, substr := range secretSubstrings {
		lowerSubstr := strings.ToLower(substr)
		if strings.Contains(lowerBase, lowerSubstr) {
			return true, fmt.Sprintf("filename contains '%s'", substr)
		}
	}

	// 3. Path check (.ssh, .aws/credentials, etc.)
	for _, pattern := range secretPathPatterns {
		lowerPattern := strings.ToLower(pattern)
		if strings.Contains(lowerPath, lowerPattern) {
			return true, fmt.Sprintf("secret path (%s)", pattern)
		}
	}

	return false, ""
}

// IsSystemPath checks if path is system (write-protected)
func IsSystemPath(path string) bool {
	cleanPath := filepath.Clean(path)

	// Resolve symlinks (on macOS /etc -> /private/etc)
	evalPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		cleanPath = evalPath
	}

	for _, sysPath := range systemPaths {
		if strings.HasPrefix(cleanPath, sysPath+string(filepath.Separator)) || cleanPath == sysPath {
			return true
		}
	}
	return false
}

// ResolvePath safely resolves path: cleans, resolves symlinks, checks ..
func ResolvePath(path string) (string, error) {
	// Clean path
	cleanPath := filepath.Clean(path)

	// Check path traversal
	if strings.Contains(cleanPath, "..") {
		return "", ErrPathTraversal
	}

	// Resolve symlinks
	evalPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		cleanPath = evalPath
	}

	return cleanPath, nil
}

// IsPathAllowed checks if path is allowed within AllowedDirs
func IsPathAllowed(path string, allowedDirs []string) bool {
	if len(allowedDirs) == 0 {
		return true
	}

	cleanPath := filepath.Clean(path)

	// Resolve symlinks only if path exists
	evalPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		cleanPath = evalPath
	}

	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		// Resolve symlinks for allowed directories
		evalDir, err := filepath.EvalSymlinks(cleanDir)
		if err == nil {
			cleanDir = evalDir
		}
		// Check match with path and its resolved version
		if strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) || cleanPath == cleanDir {
			return true
		}
		// Also check with unresolved path (for non-existent files)
		if strings.HasPrefix(filepath.Clean(path), filepath.Clean(dir)+string(filepath.Separator)) {
			return true
		}
		if filepath.Clean(path) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// Security errors
var (
	ErrPathTraversal  = &SecurityError{Msg: "path contains invalid components (..)"}
	ErrSecretPath     = &SecurityError{Msg: "access to secret file denied"}
	ErrSystemPath     = &SecurityError{Msg: "writing to system path denied"}
	ErrPathNotAllowed = &SecurityError{Msg: "access to file denied"}
)

// SecurityError — error security
type SecurityError struct {
	Msg string
}

func (e *SecurityError) Error() string {
	return e.Msg
}
