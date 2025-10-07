package domain

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/ast"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

var (
	workflowStateNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	workflowStatePathRegex = regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`)
)

const (
	workflowStateNameMaxLength = 64
	workflowStatePathMaxLength = 255
	workflowStateMaxDepth      = 5
)

const workflowSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Workflow Definition",
  "type": "object",
  "required": ["state_directories", "initial_state", "states"],
  "properties": {
    "state_directories": {
      "type": "object",
      "description": "Mapping of state names to directory paths (relative to workflow home)",
      "minProperties": 1,
      "patternProperties": {
        "^[a-z0-9][a-z0-9_-]*$": {
          "type": "string",
          "pattern": "^[a-zA-Z0-9_/-]+$",
          "minLength": 1,
          "maxLength": 255
        }
      },
      "additionalProperties": false
    },
    "initial_state": {
      "type": "string",
      "description": "State where files must be created",
      "pattern": "^[a-z0-9][a-z0-9_-]*$"
    },
    "states": {
      "type": "object",
      "description": "State definitions with allowed transitions",
      "minProperties": 1,
      "patternProperties": {
        "^[a-z0-9][a-z0-9_-]*$": {
          "type": "object",
          "properties": {
            "transitions": {
              "type": "array",
              "description": "Valid target states for transitions",
              "items": {
                "type": "object",
                "required": ["to"],
                "properties": {
                  "to": {
                    "type": "string",
                    "pattern": "^[a-z0-9][a-z0-9_-]*$",
                    "description": "Target state name"
                  },
                  "description": {
                    "type": "string",
                    "description": "Human-readable description of this transition"
                  }
                }
              }
            }
          }
        }
      },
      "additionalProperties": false
    },
    "gate_policy": {
      "type": "string",
      "description": "Inline Rego policy for gate evaluation (package vfs.workflow.gates)"
    },
    "gate_policy_ref": {
      "type": "string",
      "description": "Path to external .rego file (relative to workflow home, e.g., '.workflow.rego')",
      "pattern": "^\\.workflow\\.rego$"
    }
  },
  "oneOf": [
    {"required": ["gate_policy"]},
    {"required": ["gate_policy_ref"]},
    {"not": {"anyOf": [{"required": ["gate_policy"]}, {"required": ["gate_policy_ref"]}]}}
  ]
}`

var (
	workflowSchemaOnce sync.Once
	workflowSchema     *jsonschema.Schema
	workflowSchemaErr  error
)

func loadWorkflowSchema() (*jsonschema.Schema, error) {
	workflowSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		workflowSchemaErr = compiler.AddResource("workflow.schema.json", strings.NewReader(workflowSchemaJSON))
		if workflowSchemaErr != nil {
			return
		}
		workflowSchema, workflowSchemaErr = compiler.Compile("workflow.schema.json")
	})

	return workflowSchema, workflowSchemaErr
}

type workflowTransitionConfig struct {
	To          string `yaml:"to"`
	Description string `yaml:"description"`
}

type workflowStateConfig struct {
	Transitions []workflowTransitionConfig `yaml:"transitions"`
}

type workflowConfig struct {
	StateDirectories map[string]string              `yaml:"state_directories"`
	InitialState     string                         `yaml:"initial_state"`
	States           map[string]workflowStateConfig `yaml:"states"`
	GatePolicy       string                         `yaml:"gate_policy"`
	GatePolicyRef    string                         `yaml:"gate_policy_ref"`
}

func validateWorkflowConfig(content []byte) error {
	_, err := decodeWorkflowConfig(content)
	return err
}

func decodeWorkflowConfig(content []byte) (*workflowConfig, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil, newWorkflowValidationError(ErrInvalidYAML, "workflow definition cannot be empty", nil)
	}

	var yamlObj map[string]interface{}
	if err := yaml.Unmarshal(trimmed, &yamlObj); err != nil {
		return nil, newWorkflowValidationError(ErrInvalidYAML, fmt.Sprintf("invalid YAML: %v", err), nil)
	}

	schema, err := loadWorkflowSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow schema: %w", err)
	}

	if err := schema.Validate(yamlObj); err != nil {
		return nil, newWorkflowValidationError(ErrSchemaViolation, err.Error(), map[string]interface{}{"schema_error": err.Error()})
	}

	var cfg workflowConfig
	if err := yaml.Unmarshal(trimmed, &cfg); err != nil {
		return nil, newWorkflowValidationError(ErrInvalidYAML, fmt.Sprintf("invalid workflow structure: %v", err), nil)
	}

	if len(cfg.StateDirectories) == 0 {
		return nil, newWorkflowValidationError(ErrSchemaViolation, "state_directories must include at least one state", nil)
	}
	if len(cfg.States) == 0 {
		return nil, newWorkflowValidationError(ErrSchemaViolation, "states must include at least one state", nil)
	}

	initialState := strings.TrimSpace(cfg.InitialState)
	if initialState == "" {
		return nil, newWorkflowValidationError(ErrInvalidStateName, "initial_state cannot be empty", nil)
	}
	if len(initialState) > workflowStateNameMaxLength {
		return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("initial_state '%s' exceeds maximum length (%d)", initialState, workflowStateNameMaxLength), map[string]interface{}{"initial_state": initialState})
	}
	if !workflowStateNameRegex.MatchString(initialState) {
		return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("initial_state '%s' must match pattern %s", initialState, workflowStateNameRegex.String()), map[string]interface{}{"initial_state": initialState})
	}
	cfg.InitialState = initialState

	for state := range cfg.States {
		if len(state) > workflowStateNameMaxLength {
			return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("state '%s' exceeds maximum length (%d)", state, workflowStateNameMaxLength), map[string]interface{}{"state": state})
		}
		if !workflowStateNameRegex.MatchString(state) {
			return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("state '%s' must match pattern %s", state, workflowStateNameRegex.String()), map[string]interface{}{"state": state})
		}
	}

	for state, dir := range cfg.StateDirectories {
		if len(state) > workflowStateNameMaxLength {
			return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("state '%s' exceeds maximum length (%d)", state, workflowStateNameMaxLength), map[string]interface{}{"state": state})
		}
		if !workflowStateNameRegex.MatchString(state) {
			return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("state '%s' must match pattern %s", state, workflowStateNameRegex.String()), map[string]interface{}{"state": state})
		}

		cleanDir := strings.TrimSpace(dir)
		if cleanDir == "" {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory cannot be empty", state), map[string]interface{}{"state": state})
		}
		if len(cleanDir) > workflowStatePathMaxLength {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory exceeds maximum length (%d)", state, workflowStatePathMaxLength), map[string]interface{}{"state": state, "path": cleanDir})
		}
		if strings.HasPrefix(cleanDir, "/") || strings.HasSuffix(cleanDir, "/") {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory must be relative without leading or trailing slash", state), map[string]interface{}{"state": state, "path": cleanDir})
		}
		if !workflowStatePathRegex.MatchString(cleanDir) {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory '%s' contains invalid characters", state, cleanDir), map[string]interface{}{"state": state, "path": cleanDir})
		}

		segments := strings.Split(cleanDir, "/")
		if len(segments) > workflowStateMaxDepth {
			return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory '%s' exceeds maximum depth of %d", state, cleanDir, workflowStateMaxDepth), map[string]interface{}{"state": state, "path": cleanDir})
		}
		for _, segment := range segments {
			if segment == "" {
				return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory '%s' cannot contain empty path segments", state, cleanDir), map[string]interface{}{"state": state, "path": cleanDir})
			}
			if segment == "." || segment == ".." {
				return nil, newWorkflowValidationError(ErrInvalidStatePath, fmt.Sprintf("state '%s' directory '%s' cannot contain '.' or '..'", state, cleanDir), map[string]interface{}{"state": state, "path": cleanDir})
			}
		}

		cfg.StateDirectories[state] = cleanDir
	}

	if _, exists := cfg.States[cfg.InitialState]; !exists {
		return nil, newWorkflowValidationError(ErrInitialStateNotFound, fmt.Sprintf("initial_state '%s' does not exist in states", cfg.InitialState), map[string]interface{}{"initial_state": cfg.InitialState})
	}

	for state := range cfg.StateDirectories {
		if _, ok := cfg.States[state]; !ok {
			return nil, newWorkflowValidationError(ErrStateDirectoryNotFound, fmt.Sprintf("state directory mapping references unknown state '%s'", state), map[string]interface{}{"state": state})
		}
	}

	for state := range cfg.States {
		if _, ok := cfg.StateDirectories[state]; !ok {
			return nil, newWorkflowValidationError(ErrOrphanedState, fmt.Sprintf("state '%s' is missing a state directory mapping", state), map[string]interface{}{"state": state})
		}
	}

	for state, def := range cfg.States {
		for idx, transition := range def.Transitions {
			target := strings.TrimSpace(transition.To)
			if target == "" {
				return nil, newWorkflowValidationError(ErrTransitionStateNotFound, fmt.Sprintf("state '%s' transition at index %d must specify a target state", state, idx), map[string]interface{}{"state": state, "transition_index": idx})
			}
			if len(target) > workflowStateNameMaxLength {
				return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("transition target '%s' exceeds maximum length (%d)", target, workflowStateNameMaxLength), map[string]interface{}{"state": state, "target": target})
			}
			if !workflowStateNameRegex.MatchString(target) {
				return nil, newWorkflowValidationError(ErrInvalidStateName, fmt.Sprintf("transition target '%s' must match pattern %s", target, workflowStateNameRegex.String()), map[string]interface{}{"state": state, "target": target})
			}
			if _, ok := cfg.States[target]; !ok {
				return nil, newWorkflowValidationError(ErrTransitionStateNotFound, fmt.Sprintf("state '%s' references unknown transition target '%s'", state, target), map[string]interface{}{"state": state, "target": target})
			}
			def.Transitions[idx].To = target
		}
		cfg.States[state] = def
	}

	inlinePolicy := strings.TrimSpace(cfg.GatePolicy)
	refPolicy := strings.TrimSpace(cfg.GatePolicyRef)

	if inlinePolicy != "" && refPolicy != "" {
		return nil, newWorkflowValidationError(ErrBothGatePolicies, "gate_policy and gate_policy_ref cannot both be defined", nil)
	}

	if inlinePolicy != "" {
		module, err := ast.ParseModule("workflow_policy.rego", inlinePolicy)
		if err != nil {
			return nil, newWorkflowValidationError(ErrInvalidGatePolicy, fmt.Sprintf("invalid gate_policy: %v", err), map[string]interface{}{"reason": err.Error()})
		}
		if module.Package == nil || module.Package.Path.String() != "vfs.workflow.gates" {
			return nil, newWorkflowValidationError(ErrInvalidGatePolicy, "gate_policy must declare package vfs.workflow.gates", nil)
		}
		hasAllowRule := false
		for _, rule := range module.Rules {
			if rule.Head != nil && rule.Head.Name == "allow" {
				hasAllowRule = true
				break
			}
		}
		if !hasAllowRule {
			return nil, newWorkflowValidationError(ErrInvalidGatePolicy, "gate_policy must define an allow rule", nil)
		}
	}

	cfg.GatePolicy = inlinePolicy
	cfg.GatePolicyRef = refPolicy

	return &cfg, nil
}
