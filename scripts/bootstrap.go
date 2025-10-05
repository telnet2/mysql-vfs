package main

import (
	"fmt"

	"github.com/telnet2/mysql-vfs/pkg/setup"
)

func main() {
	fmt.Println("🚀 MySQL VFS Bootstrap")
	fmt.Println("=======================")
	fmt.Println()
	fmt.Println("This script helps you bootstrap your MySQL VFS instance.")
	fmt.Println()
	fmt.Println("📋 Default files to create:")
	fmt.Println()
	fmt.Println("1. /.rego - Authorization policy")
	fmt.Println("---")
	fmt.Println(setup.DefaultRegoPolicy)
	fmt.Println()
	fmt.Println("2. /.group - Group definitions")
	fmt.Println("---")
	fmt.Println(setup.DefaultGroupConfig)
	fmt.Println()
	fmt.Println("💡 To create these files, use the API:")
	fmt.Println()
	fmt.Println("  # Create /.rego")
	fmt.Println("  curl -X POST http://localhost:8080/api/v1/files \\")
	fmt.Println("    -H \"Authorization: Bearer <system-admin-token>\" \\")
	fmt.Println("    -d '{")
	fmt.Println("      \"directory_path\": \"/\",")
	fmt.Println("      \"name\": \".rego\",")
	fmt.Println("      \"content_type\": \"text/plain\",")
	fmt.Println("      \"content\": \"<paste .rego content>\"")
	fmt.Println("    }'")
	fmt.Println()
	fmt.Println("  # Create /.group")
	fmt.Println("  curl -X POST http://localhost:8080/api/v1/files \\")
	fmt.Println("    -H \"Authorization: Bearer <system-admin-token>\" \\")
	fmt.Println("    -d '{")
	fmt.Println("      \"directory_path\": \"/\",")
	fmt.Println("      \"name\": \".group\",")
	fmt.Println("      \"content_type\": \"application/json\",")
	fmt.Println("      \"content\": \"<paste .group content>\"")
	fmt.Println("    }'")
	fmt.Println()
	fmt.Println("📖 See BOOTSTRAP.md for detailed instructions")
}
