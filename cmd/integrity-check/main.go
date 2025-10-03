package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"gorm.io/gorm/logger"

	"github.com/telnet2/mysql-vfs/pkg/db"
	"github.com/telnet2/mysql-vfs/pkg/integrity"
)

func main() {
	// Parse flags
	dsn := flag.String("dsn", "", "Database DSN (required)")
	repair := flag.Bool("repair", false, "Perform repairs (default: false - check only)")
	dryRun := flag.Bool("dry-run", true, "Dry run mode for repairs (default: true)")
	verbose := flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	if *dsn == "" {
		*dsn = os.Getenv("DB_DSN")
		if *dsn == "" {
			fmt.Println("Error: Database DSN is required")
			fmt.Println("Usage: integrity-check -dsn <connection-string> [-repair] [-dry-run=false] [-verbose]")
			os.Exit(1)
		}
	}

	// Connect to database
	logLevel := logger.Warn
	if *verbose {
		logLevel = logger.Info
	}

	database, err := db.Connect(db.Config{
		DSN:      *dsn,
		LogLevel: logLevel,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	ctx := context.Background()

	fmt.Println("=== VFS Referential Integrity Check ===")
	fmt.Printf("Mode: %s\n", getMode(*repair, *dryRun))
	fmt.Println()

	if *repair {
		// Run repair
		repairService := integrity.NewRepairService(database)
		results, err := repairService.RepairAll(ctx, *dryRun)
		if err != nil {
			log.Fatalf("Repair failed: %v", err)
		}

		// Display results
		if len(results) == 0 {
			fmt.Println("✓ No violations found. Database integrity is good!")
		} else {
			fmt.Printf("Found %d violations:\n\n", len(results))
			displayRepairResults(results)
		}
	} else {
		// Run validation only
		validator := integrity.NewValidator(database)
		results, err := validator.ValidateAll(ctx)
		if err != nil {
			log.Fatalf("Validation failed: %v", err)
		}

		// Display results
		if len(results) == 0 {
			fmt.Println("✓ No violations found. Database integrity is good!")
		} else {
			fmt.Printf("Found %d violations:\n\n", len(results))
			displayValidationResults(results)

			fmt.Println()
			fmt.Println("To repair these violations, run with -repair flag:")
			fmt.Println("  integrity-check -dsn <dsn> -repair")
			fmt.Println()
			fmt.Println("Use -dry-run=false to apply repairs:")
			fmt.Println("  integrity-check -dsn <dsn> -repair -dry-run=false")
		}
	}

	fmt.Println()
	fmt.Println("Integrity check complete.")
}

func getMode(repair, dryRun bool) string {
	if !repair {
		return "validation only (no repairs)"
	}
	if dryRun {
		return "repair (dry run - no changes will be made)"
	}
	return "repair (changes will be applied)"
}

func displayValidationResults(results []integrity.ValidationResult) {
	// Group by table
	byTable := make(map[string][]integrity.ValidationResult)
	for _, r := range results {
		byTable[r.TableName] = append(byTable[r.TableName], r)
	}

	for table, violations := range byTable {
		fmt.Printf("Table: %s (%d violations)\n", table, len(violations))
		for i, v := range violations {
			fmt.Printf("  %d. [%s] %s\n", i+1, v.ViolationType, v.Description)
			if v.RecordID != "" {
				fmt.Printf("     Record ID: %s\n", v.RecordID)
			}
		}
		fmt.Println()
	}
}

func displayRepairResults(results []integrity.RepairResult) {
	successCount := 0
	failureCount := 0

	for _, r := range results {
		symbol := "✓"
		if !r.Success {
			symbol = "✗"
			failureCount++
		} else {
			successCount++
		}

		fmt.Printf("%s [%s] %s\n", symbol, r.ViolationType, r.Action)
		if r.Error != nil {
			fmt.Printf("  Error: %v\n", r.Error)
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d successful, %d failed\n", successCount, failureCount)
}
