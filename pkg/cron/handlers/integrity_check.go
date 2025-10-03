package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/telnet2/mysql-vfs/pkg/integrity"
)

// IntegrityCheckHandler performs periodic integrity checks
type IntegrityCheckHandler struct {
	db *gorm.DB
}

// NewIntegrityCheckHandler creates a new integrity check handler
func NewIntegrityCheckHandler(db *gorm.DB) *IntegrityCheckHandler {
	return &IntegrityCheckHandler{db: db}
}

// IntegrityCheckPayload is the configuration for integrity check jobs
type IntegrityCheckPayload struct {
	AutoRepair bool `json:"auto_repair"`
	DryRun     bool `json:"dry_run"`
}

// Execute runs the integrity check
func (h *IntegrityCheckHandler) Execute(ctx context.Context, payload string) error {
	log.Println("Starting periodic integrity check...")

	// Parse payload
	var config IntegrityCheckPayload
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &config); err != nil {
			return fmt.Errorf("failed to parse payload: %w", err)
		}
	} else {
		// Default: check only, no repairs
		config.AutoRepair = false
		config.DryRun = true
	}

	if config.AutoRepair {
		// Run repairs
		repairService := integrity.NewRepairService(h.db)
		results, err := repairService.RepairAll(ctx, config.DryRun)
		if err != nil {
			return fmt.Errorf("repair failed: %w", err)
		}

		successCount := 0
		for _, r := range results {
			if r.Success {
				successCount++
			}
		}

		log.Printf("Integrity repair complete. %d/%d operations successful.\n", successCount, len(results))

		if len(results) > 0 {
			// Log details of repairs
			for _, r := range results {
				if r.Success {
					log.Printf("  ✓ [%s] %s\n", r.ViolationType, r.Action)
				} else {
					log.Printf("  ✗ [%s] %s: %v\n", r.ViolationType, r.Action, r.Error)
				}
			}
		}
	} else {
		// Run validation only
		validator := integrity.NewValidator(h.db)
		results, err := validator.ValidateAll(ctx)
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}

		if len(results) == 0 {
			log.Println("✓ No integrity violations found.")
		} else {
			log.Printf("⚠ Found %d integrity violations:\n", len(results))

			// Group by table
			byTable := make(map[string][]integrity.ValidationResult)
			for _, r := range results {
				byTable[r.TableName] = append(byTable[r.TableName], r)
			}

			for table, violations := range byTable {
				log.Printf("  %s: %d violations\n", table, len(violations))
				for _, v := range violations {
					log.Printf("    - [%s] %s\n", v.ViolationType, v.Description)
				}
			}

			log.Println("To enable auto-repair, set auto_repair=true in job payload")
		}
	}

	return nil
}

// Name returns the handler name
func (h *IntegrityCheckHandler) Name() string {
	return "integrity_check"
}
