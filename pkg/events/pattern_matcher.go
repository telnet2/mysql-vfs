package events

import (
	"fmt"
	"regexp"
	"strings"
)

// WildcardPatternMatcher implements pattern matching for event types
// Supports wildcards like:
// - file.create.*                    (all stages of file.create)
// - file.*.authorization.*           (authorization for all file ops)
// - *.*.validation.failed            (all validation failures)
// - file.{create,update}.*          (multiple operations)
type WildcardPatternMatcher struct{}

// NewWildcardPatternMatcher creates a new wildcard pattern matcher
func NewWildcardPatternMatcher() *WildcardPatternMatcher {
	return &WildcardPatternMatcher{}
}

// Match checks if an event type matches a pattern
func (m *WildcardPatternMatcher) Match(pattern string, eventType string) bool {
	// Exact match
	if pattern == eventType {
		return true
	}

	// Wildcard match
	return m.matchWildcard(pattern, eventType)
}

// matchWildcard performs wildcard matching
func (m *WildcardPatternMatcher) matchWildcard(pattern string, eventType string) bool {
	// Handle brace expansion: file.{create,update}.* -> file.create.* | file.update.*
	if strings.Contains(pattern, "{") {
		expanded := m.expandBraces(pattern)
		for _, p := range expanded {
			if m.matchSimpleWildcard(p, eventType) {
				return true
			}
		}
		return false
	}

	return m.matchSimpleWildcard(pattern, eventType)
}

// matchSimpleWildcard matches a pattern with * wildcards (no braces)
func (m *WildcardPatternMatcher) matchSimpleWildcard(pattern string, eventType string) bool {
	patternParts := strings.Split(pattern, ".")
	eventParts := strings.Split(eventType, ".")

	// If pattern has no wildcards and lengths differ, no match
	if !strings.Contains(pattern, "*") && len(patternParts) != len(eventParts) {
		return false
	}

	// Match each part
	return m.matchParts(patternParts, eventParts)
}

// matchParts matches pattern parts against event parts
func (m *WildcardPatternMatcher) matchParts(patternParts []string, eventParts []string) bool {
	pi := 0 // pattern index
	ei := 0 // event index

	for pi < len(patternParts) && ei < len(eventParts) {
		patternPart := patternParts[pi]

		if patternPart == "*" {
			// Single wildcard matches exactly one part
			pi++
			ei++
		} else if patternPart == "**" {
			// Double wildcard matches zero or more parts
			// Try to match the rest of the pattern
			if pi == len(patternParts)-1 {
				// ** at the end matches everything remaining
				return true
			}

			// Try matching the next pattern part at different positions
			for i := ei; i <= len(eventParts); i++ {
				if m.matchParts(patternParts[pi+1:], eventParts[i:]) {
					return true
				}
			}
			return false
		} else {
			// Literal match
			if patternPart != eventParts[ei] {
				return false
			}
			pi++
			ei++
		}
	}

	// Check if both are exhausted
	// If pattern has trailing *, consume them
	for pi < len(patternParts) && patternParts[pi] == "*" {
		pi++
		ei++
	}

	return pi == len(patternParts) && ei == len(eventParts)
}

// expandBraces expands brace notation: file.{create,update}.* -> [file.create.*, file.update.*]
func (m *WildcardPatternMatcher) expandBraces(pattern string) []string {
	// Find first brace group
	start := strings.Index(pattern, "{")
	if start == -1 {
		return []string{pattern}
	}

	end := strings.Index(pattern[start:], "}")
	if end == -1 {
		return []string{pattern}
	}
	end += start

	// Extract prefix, options, and suffix
	prefix := pattern[:start]
	optionsStr := pattern[start+1 : end]
	suffix := pattern[end+1:]

	options := strings.Split(optionsStr, ",")

	// Expand each option
	var results []string
	for _, opt := range options {
		expanded := prefix + strings.TrimSpace(opt) + suffix
		// Recursively expand remaining braces
		results = append(results, m.expandBraces(expanded)...)
	}

	return results
}

// CompilePattern compiles a pattern into a regex for faster matching
func (m *WildcardPatternMatcher) CompilePattern(pattern string) (interface{}, error) {
	// Convert wildcard pattern to regex
	regexPattern := m.wildcardToRegex(pattern)

	re, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern %s: %w", pattern, err)
	}

	return re, nil
}

// MatchCompiled matches using a pre-compiled regex
func (m *WildcardPatternMatcher) MatchCompiled(compiled interface{}, eventType string) bool {
	re, ok := compiled.(*regexp.Regexp)
	if !ok {
		return false
	}
	return re.MatchString(eventType)
}

// wildcardToRegex converts a wildcard pattern to a regex pattern
func (m *WildcardPatternMatcher) wildcardToRegex(pattern string) string {
	// Expand braces first
	if strings.Contains(pattern, "{") {
		expanded := m.expandBraces(pattern)
		if len(expanded) == 1 {
			pattern = expanded[0]
		} else {
			// Create alternation: (file\.create|file\.update)
			var regexParts []string
			for _, p := range expanded {
				regexParts = append(regexParts, m.wildcardToRegexSimple(p))
			}
			return "(" + strings.Join(regexParts, "|") + ")"
		}
	}

	return m.wildcardToRegexSimple(pattern)
}

// wildcardToRegexSimple converts a simple wildcard pattern (no braces) to regex
func (m *WildcardPatternMatcher) wildcardToRegexSimple(pattern string) string {
	// Escape regex special characters except * and .
	pattern = regexp.QuoteMeta(pattern)

	// Replace escaped \* with regex .*
	pattern = strings.ReplaceAll(pattern, "\\*", "[^.]+")

	return pattern
}
