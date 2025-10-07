package defaults

import _ "embed"

//go:embed _rego
var defaultRego []byte

//go:embed _group
var defaultGroup []byte

// DefaultRego returns the embedded default .rego policy.
func DefaultRego() []byte {
	return defaultRego
}

// DefaultGroup returns the embedded default .group configuration.
func DefaultGroup() []byte {
	return defaultGroup
}
