package session

import (
	"path"
	"strings"
)

// Session manages CLI session state
type Session struct {
	currentDirectory string
	authToken        string
}

// NewSession creates a new session
func NewSession() *Session {
	return &Session{
		currentDirectory: "/",
	}
}

// GetCurrentDirectory returns the current directory path
func (s *Session) GetCurrentDirectory() string {
	return s.currentDirectory
}

// SetCurrentDirectory sets the current directory path
func (s *Session) SetCurrentDirectory(dir string) {
	s.currentDirectory = dir
}

// GetAuthToken returns the authentication token
func (s *Session) GetAuthToken() string {
	return s.authToken
}

// SetAuthToken sets the authentication token
func (s *Session) SetAuthToken(token string) {
	s.authToken = token
}

// ResolvePath resolves a path relative to the current directory
func (s *Session) ResolvePath(inputPath string) string {
	// If absolute path, return as-is
	if strings.HasPrefix(inputPath, "/") {
		return path.Clean(inputPath)
	}

	// Relative path - resolve relative to current directory
	fullPath := path.Join(s.currentDirectory, inputPath)
	return path.Clean(fullPath)
}

// IsValidPath checks if a path is valid (no .. traversal beyond root)
func IsValidPath(p string) bool {
	// Must start with /
	if !strings.HasPrefix(p, "/") {
		return false
	}

	// Check for .. before cleaning
	if strings.Contains(p, "..") {
		return false
	}

	cleaned := path.Clean(p)

	// After cleaning, must still start with /
	if !strings.HasPrefix(cleaned, "/") {
		return false
	}

	return true
}
