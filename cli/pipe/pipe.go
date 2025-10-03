package pipe

import (
	"io"
	"strings"
)

// CommandSegment represents a single command in a pipe
type CommandSegment struct {
	Command string
	Args    []string
}

// ParsePipeline parses a command line into segments separated by pipes
func ParsePipeline(input string) []CommandSegment {
	segments := []CommandSegment{}

	// Split by pipe
	parts := strings.Split(input, "|")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Parse command and args
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}

		segments = append(segments, CommandSegment{
			Command: fields[0],
			Args:    fields[1:],
		})
	}

	return segments
}

// PipeReader creates a pipe pair for streaming data between commands
func PipeReader() (io.Reader, io.WriteCloser) {
	r, w := io.Pipe()
	return r, w
}
