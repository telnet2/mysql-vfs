package domain

// AuthContext holds authentication and authorization context
// This mirrors pkg/middleware.AuthContext to avoid import cycles
// The middleware layer will convert between the two types
type AuthContext struct {
	UserID   string
	Groups   []string
	Metadata map[string]interface{}

	// On-behalf-of delegation fields
	PrincipalUserID  string
	DelegationReason string
	RequestID        string
}

// GetOwner returns the effective owner (principal if delegated, otherwise actor)
func (a *AuthContext) GetOwner() string {
	if a.PrincipalUserID != "" {
		return a.PrincipalUserID
	}
	return a.UserID
}

// GetCreator returns the actual creator (always the actor)
func (a *AuthContext) GetCreator() string {
	return a.UserID
}

// IsDelegated returns true if this is a delegated request
func (a *AuthContext) IsDelegated() bool {
	return a.PrincipalUserID != "" && a.PrincipalUserID != a.UserID
}
