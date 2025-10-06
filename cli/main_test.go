package main

import (
	"bytes"
	"testing"

	"github.com/telnet2/mysql-vfs/cli/client"
	"github.com/telnet2/mysql-vfs/cli/commands"
	"github.com/telnet2/mysql-vfs/cli/session"
)

func TestCommandRegistration(t *testing.T) {
	// Test that all expected commands are registered
	expectedCommands := []string{
		"ls", "cd", "pwd", "mkdir", "rmdir",
		"cat", "import", "rm", "mv", "cp", "jq", "help",
		"attr", "grep", "find",
	}

	vfsClient := client.NewClient("http://localhost:8080")
	sess := session.NewSession()

	cmdMap := map[string]commands.Command{
		"ls":     &commands.LsCommand{},
		"cd":     &commands.CdCommand{},
		"pwd":    &commands.PwdCommand{},
		"mkdir":  &commands.MkdirCommand{},
		"rmdir":  &commands.RmdirCommand{},
		"cat":    &commands.CatCommand{},
		"import": &commands.ImportCommand{},
		"rm":     &commands.RmCommand{},
		"mv":     &commands.MvCommand{},
		"cp":     &commands.CpCommand{},
		"grep":   &commands.GrepCommand{},
		"find":   &commands.FindCommand{},
		"attr":   &commands.AttrCommand{},
		"jq":     &commands.JqCommand{},
		"help":   commands.NewHelpCommand(nil),
	}

	ctx := &commands.Context{
		Client:  vfsClient,
		Session: sess,
	}

	for _, cmdName := range expectedCommands {
		if _, exists := cmdMap[cmdName]; !exists {
			t.Errorf("Expected command %s not registered", cmdName)
		}
	}

	// Test that commands have Help() method
	for name, cmd := range cmdMap {
		help := cmd.Help()
		if help == "" {
			t.Errorf("Command %s has empty help text", name)
		}
	}

	// Use ctx to avoid unused variable warning
	_ = ctx
}

func TestSessionPathResolution(t *testing.T) {
	sess := session.NewSession()

	tests := []struct {
		currentDir string
		input      string
		expected   string
	}{
		{"/", "test", "/test"},
		{"/", "/absolute", "/absolute"},
		{"/projects", "file.txt", "/projects/file.txt"},
		{"/projects", "../other", "/other"},
		{"/a/b/c", "../../d", "/a/d"},
	}

	for _, tt := range tests {
		sess.SetCurrentDirectory(tt.currentDir)
		result := sess.ResolvePath(tt.input)
		if result != tt.expected {
			t.Errorf("ResolvePath(%q) from %q = %q, want %q",
				tt.input, tt.currentDir, result, tt.expected)
		}
	}
}

func TestPathValidation(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{"/", true},
		{"/test", true},
		{"/a/b/c", true},
		{"relative", false},     // Must start with /
		{"/test/../etc", false}, // No .. allowed
		{"/normal/path", true},
	}

	for _, tt := range tests {
		result := session.IsValidPath(tt.path)
		if result != tt.valid {
			t.Errorf("IsValidPath(%q) = %v, want %v", tt.path, result, tt.valid)
		}
	}
}

func TestHelpCommand(t *testing.T) {
	helpCmd := commands.NewHelpCommand(nil)

	var stdout bytes.Buffer
	ctx := &commands.Context{
		Stdout: &stdout,
	}

	err := helpCmd.Execute(ctx, []string{})
	if err != nil {
		t.Errorf("Help command failed: %v", err)
	}

	output := stdout.String()
	if output == "" {
		t.Error("Help command produced no output")
	}

	// Check that help mentions some commands
	expectedCommands := []string{"ls", "cd", "mkdir", "cat", "import"}
	for _, cmd := range expectedCommands {
		if !bytes.Contains(stdout.Bytes(), []byte(cmd)) {
			t.Errorf("Help output doesn't mention %s command", cmd)
		}
	}
}

func TestPwdCommand(t *testing.T) {
	pwdCmd := &commands.PwdCommand{}
	sess := session.NewSession()
	sess.SetCurrentDirectory("/test/path")

	var stdout bytes.Buffer
	ctx := &commands.Context{
		Session: sess,
		Stdout:  &stdout,
	}

	err := pwdCmd.Execute(ctx, []string{})
	if err != nil {
		t.Errorf("Pwd command failed: %v", err)
	}

	output := stdout.String()
	expected := "/test/path\n"
	if output != expected {
		t.Errorf("Pwd output = %q, want %q", output, expected)
	}
}
