package cmd

import (
	"fmt"
	"strings"
)

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"

	ColorBold      = "\033[1m"
	ColorUnderline = "\033[4m"
)

// Colorize adds color to text
func Colorize(color, text string) string {
	return color + text + ColorReset
}

// Error formats an error message in red
func ErrorMsg(msg string) string {
	return Colorize(ColorRed, "✗ "+msg)
}

// Success formats a success message in green
func SuccessMsg(msg string) string {
	return Colorize(ColorGreen, "✓ "+msg)
}

// Warning formats a warning message in yellow
func WarningMsg(msg string) string {
	return Colorize(ColorYellow, "⚠ "+msg)
}

// Info formats an info message in cyan
func InfoMsg(msg string) string {
	return Colorize(ColorCyan, "ℹ "+msg)
}

// Command formats a command name in bold cyan
func CommandName(cmd string) string {
	return Colorize(ColorBold+ColorCyan, cmd)
}

// Arg formats an argument in yellow
func ArgName(arg string) string {
	return Colorize(ColorYellow, arg)
}

// Flag formats a flag in green
func FlagName(flag string) string {
	return Colorize(ColorGreen, flag)
}

// FormatREPLUsage formats a usage message for REPL mode
func FormatREPLUsage(command, usage string, flags map[string]string, examples []string) string {
	var sb strings.Builder

	// Usage line
	sb.WriteString(Colorize(ColorBold, "Usage: "))
	sb.WriteString(CommandName(command))
	sb.WriteString(" ")
	sb.WriteString(usage)
	sb.WriteString("\n")

	// Flags (if any)
	if len(flags) > 0 {
		sb.WriteString("\n")
		sb.WriteString(Colorize(ColorBold, "Flags:\n"))
		for flag, desc := range flags {
			sb.WriteString("  ")
			sb.WriteString(FlagName(flag))
			sb.WriteString("  ")
			sb.WriteString(Colorize(ColorGray, desc))
			sb.WriteString("\n")
		}
	}

	// Examples (if any)
	if len(examples) > 0 {
		sb.WriteString("\n")
		sb.WriteString(Colorize(ColorBold, "Examples:\n"))
		for _, example := range examples {
			sb.WriteString("  ")
			sb.WriteString(Colorize(ColorCyan, example))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatREPLError formats an error for REPL display
func FormatREPLError(err error, command string) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Check if it's a flag parsing error
	if strings.Contains(errMsg, "unknown flag") || strings.Contains(errMsg, "unknown shorthand flag") {
		// Extract the flag name
		var flagName string
		if strings.Contains(errMsg, "unknown shorthand flag:") {
			parts := strings.Split(errMsg, "'")
			if len(parts) >= 2 {
				flagName = parts[1]
			}
		} else if strings.Contains(errMsg, "unknown flag:") {
			parts := strings.Split(errMsg, ":")
			if len(parts) >= 2 {
				flagName = strings.TrimSpace(parts[1])
			}
		}

		var sb strings.Builder
		sb.WriteString(ErrorMsg(fmt.Sprintf("Unknown flag: %s", FlagName(flagName))))
		sb.WriteString("\n")
		sb.WriteString(Colorize(ColorGray, fmt.Sprintf("Type '%s' to see available options", Colorize(ColorCyan, "help "+command))))
		return sb.String()
	}

	// Check if it's a usage error
	if strings.Contains(errMsg, "requires") || strings.Contains(errMsg, "usage:") {
		return ErrorMsg(errMsg)
	}

	// Default error formatting
	return ErrorMsg(errMsg)
}
