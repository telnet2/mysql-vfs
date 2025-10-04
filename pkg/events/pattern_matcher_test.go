package events

import (
	"testing"
)

func TestWildcardPatternMatcher_Match(t *testing.T) {
	matcher := NewWildcardPatternMatcher()

	tests := []struct {
		name      string
		pattern   string
		eventType string
		want      bool
	}{
		// Exact matches
		{
			name:      "exact match",
			pattern:   "file.create.authorization.started",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "exact mismatch",
			pattern:   "file.create.authorization.started",
			eventType: "file.create.authorization.failed",
			want:      false,
		},

		// Single wildcard matches
		{
			name:      "wildcard at end",
			pattern:   "file.create.*",
			eventType: "file.create.authorization",
			want:      true,
		},
		{
			name:      "wildcard at end with deep path",
			pattern:   "file.create.*",
			eventType: "file.create.authorization.started",
			want:      false, // * matches exactly one segment
		},
		{
			name:      "wildcard in middle",
			pattern:   "file.*.authorization.started",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "wildcard in middle - update operation",
			pattern:   "file.*.authorization.started",
			eventType: "file.update.authorization.started",
			want:      true,
		},
		{
			name:      "wildcard in middle - wrong category",
			pattern:   "file.*.authorization.started",
			eventType: "directory.create.authorization.started",
			want:      false,
		},
		{
			name:      "multiple wildcards",
			pattern:   "*.*.authorization.started",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "multiple wildcards - directory",
			pattern:   "*.*.authorization.started",
			eventType: "directory.delete.authorization.started",
			want:      true,
		},

		// All stages
		{
			name:      "all stages of an operation",
			pattern:   "file.create.*",
			eventType: "file.create.authorization",
			want:      true,
		},
		{
			name:      "all stages of an operation - validation",
			pattern:   "file.create.*",
			eventType: "file.create.validation",
			want:      true,
		},
		{
			name:      "all stages of an operation - completion",
			pattern:   "file.create.*",
			eventType: "file.create.completion",
			want:      true,
		},

		// All operations
		{
			name:      "all file operations - authorization",
			pattern:   "file.*.authorization.*",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "all file operations - authorization for update",
			pattern:   "file.*.authorization.*",
			eventType: "file.update.authorization.succeeded",
			want:      true,
		},
		{
			name:      "all file operations - wrong stage",
			pattern:   "file.*.authorization.*",
			eventType: "file.create.validation.started",
			want:      false,
		},

		// All validation failures
		{
			name:      "all validation failures",
			pattern:   "*.*.validation.failed",
			eventType: "file.create.validation.failed",
			want:      true,
		},
		{
			name:      "all validation failures - directory",
			pattern:   "*.*.validation.failed",
			eventType: "directory.create.validation.failed",
			want:      true,
		},
		{
			name:      "all validation failures - success not matched",
			pattern:   "*.*.validation.failed",
			eventType: "file.create.validation.succeeded",
			want:      false,
		},

		// All completions
		{
			name:      "all completions",
			pattern:   "*.*.completed",
			eventType: "file.create.completed",
			want:      true,
		},
		{
			name:      "all completions - directory",
			pattern:   "*.*.completed",
			eventType: "directory.delete.completed",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.pattern, tt.eventType)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
			}
		})
	}
}

func TestWildcardPatternMatcher_BraceExpansion(t *testing.T) {
	matcher := NewWildcardPatternMatcher()

	tests := []struct {
		name      string
		pattern   string
		eventType string
		want      bool
	}{
		{
			name:      "brace expansion - first option",
			pattern:   "file.{create,update}.*",
			eventType: "file.create.authorization",
			want:      true,
		},
		{
			name:      "brace expansion - second option",
			pattern:   "file.{create,update}.*",
			eventType: "file.update.validation",
			want:      true,
		},
		{
			name:      "brace expansion - not matched",
			pattern:   "file.{create,update}.*",
			eventType: "file.delete.authorization",
			want:      false,
		},
		{
			name:      "brace expansion - multiple stages",
			pattern:   "file.create.{authorization,validation}.*",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "brace expansion - validation stage",
			pattern:   "file.create.{authorization,validation}.*",
			eventType: "file.create.validation.failed",
			want:      true,
		},
		{
			name:      "brace expansion - not matched stage",
			pattern:   "file.create.{authorization,validation}.*",
			eventType: "file.create.execution.started",
			want:      false,
		},
		{
			name:      "brace expansion with wildcards",
			pattern:   "*.{create,delete}.authorization.*",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "brace expansion with wildcards - directory",
			pattern:   "*.{create,delete}.authorization.*",
			eventType: "directory.delete.authorization.succeeded",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.pattern, tt.eventType)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
			}
		})
	}
}

func TestWildcardPatternMatcher_ExpandBraces(t *testing.T) {
	matcher := NewWildcardPatternMatcher()

	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "simple brace expansion",
			pattern: "file.{create,update}.*",
			want:    []string{"file.create.*", "file.update.*"},
		},
		{
			name:    "three options",
			pattern: "file.{create,update,delete}.completed",
			want:    []string{"file.create.completed", "file.update.completed", "file.delete.completed"},
		},
		{
			name:    "no braces",
			pattern: "file.create.*",
			want:    []string{"file.create.*"},
		},
		{
			name:    "multiple brace groups - fully expanded",
			pattern: "file.{create,update}.{authorization,validation}.*",
			want: []string{
				"file.create.authorization.*",
				"file.create.validation.*",
				"file.update.authorization.*",
				"file.update.validation.*",
			},
		},
		{
			name:    "spaces in options",
			pattern: "file.{ create , update }.*",
			want:    []string{"file.create.*", "file.update.*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.expandBraces(tt.pattern)
			if len(got) != len(tt.want) {
				t.Errorf("expandBraces(%q) returned %d results, want %d", tt.pattern, len(got), len(tt.want))
				t.Errorf("Got: %v", got)
				t.Errorf("Want: %v", tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expandBraces(%q)[%d] = %q, want %q", tt.pattern, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWildcardPatternMatcher_CompilePattern(t *testing.T) {
	matcher := NewWildcardPatternMatcher()

	tests := []struct {
		name      string
		pattern   string
		eventType string
		want      bool
		wantErr   bool
	}{
		{
			name:      "compile simple wildcard",
			pattern:   "file.create.*",
			eventType: "file.create.authorization",
			want:      true,
		},
		{
			name:      "compile multiple wildcards",
			pattern:   "*.*.authorization.started",
			eventType: "file.create.authorization.started",
			want:      true,
		},
		{
			name:      "compile brace expansion",
			pattern:   "file.{create,update}.*",
			eventType: "file.create.validation",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := matcher.CompilePattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompilePattern(%q) error = %v, wantErr %v", tt.pattern, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			got := matcher.MatchCompiled(compiled, tt.eventType)
			if got != tt.want {
				t.Errorf("MatchCompiled(compiled(%q), %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
			}
		})
	}
}

func BenchmarkWildcardPatternMatcher_Match(b *testing.B) {
	matcher := NewWildcardPatternMatcher()
	pattern := "file.*.authorization.*"
	eventType := "file.create.authorization.started"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(pattern, eventType)
	}
}

func BenchmarkWildcardPatternMatcher_MatchCompiled(b *testing.B) {
	matcher := NewWildcardPatternMatcher()
	pattern := "file.*.authorization.*"
	eventType := "file.create.authorization.started"
	compiled, _ := matcher.CompilePattern(pattern)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.MatchCompiled(compiled, eventType)
	}
}
