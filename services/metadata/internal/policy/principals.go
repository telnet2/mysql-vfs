package policy

import (
	"encoding/json"
	"fmt"
	"strings"
)

type UserPrincipal struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Email       string         `json:"email,omitempty"`
	Groups      []string       `json:"groups,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type GroupPrincipal struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name,omitempty"`
	Description string         `json:"description,omitempty"`
	Members     []string       `json:"members,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type PrincipalSet struct {
	Users  []UserPrincipal  `json:"users,omitempty"`
	Groups []GroupPrincipal `json:"groups,omitempty"`
}

type principalDocument struct {
	Scope       string           `json:"scope"`
	Inheritance string           `json:"inheritance"`
	Users       []UserPrincipal  `json:"users"`
	Groups      []GroupPrincipal `json:"groups"`
	Metadata    map[string]any   `json:"metadata"`
}

type ManifestOverrides struct {
	Scope       *Scope
	Inheritance *InheritanceMode
}

func ParsePrincipalManifest(payload []byte, manifestType Type) (PrincipalSet, ManifestOverrides, error) {
	var doc principalDocument
	if len(strings.TrimSpace(string(payload))) == 0 {
		return PrincipalSet{}, ManifestOverrides{}, fmt.Errorf("policy: empty principal manifest")
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return PrincipalSet{}, ManifestOverrides{}, fmt.Errorf("policy: invalid principal manifest: %w", err)
	}

	result := PrincipalSet{}
	if manifestType == TypeUser {
		users, err := normalizeUsers(doc.Users)
		if err != nil {
			return PrincipalSet{}, ManifestOverrides{}, err
		}
		result.Users = users
	} else if manifestType == TypeGroup {
		groups, err := normalizeGroups(doc.Groups)
		if err != nil {
			return PrincipalSet{}, ManifestOverrides{}, err
		}
		result.Groups = groups
	} else {
		return PrincipalSet{}, ManifestOverrides{}, fmt.Errorf("policy: unsupported principal manifest type %q", manifestType)
	}

	overrides := ManifestOverrides{}
	if scope, ok := parseScope(doc.Scope); ok {
		overrides.Scope = &scope
	}
	if inh, ok := parseInheritance(doc.Inheritance); ok {
		overrides.Inheritance = &inh
	}

	return result, overrides, nil
}

func normalizeUsers(users []UserPrincipal) ([]UserPrincipal, error) {
	seen := make(map[string]struct{})
	result := make([]UserPrincipal, 0, len(users))
	for _, u := range users {
		id := strings.TrimSpace(u.ID)
		if id == "" {
			return nil, fmt.Errorf("policy: user manifest missing id")
		}
		idLower := strings.ToLower(id)
		if _, exists := seen[idLower]; exists {
			return nil, fmt.Errorf("policy: duplicate user id %q", id)
		}
		seen[idLower] = struct{}{}
		u.ID = id
		u.Groups = normalizeList(u.Groups)
		result = append(result, u)
	}
	return result, nil
}

func normalizeGroups(groups []GroupPrincipal) ([]GroupPrincipal, error) {
	seen := make(map[string]struct{})
	result := make([]GroupPrincipal, 0, len(groups))
	for _, g := range groups {
		id := strings.TrimSpace(g.ID)
		if id == "" {
			return nil, fmt.Errorf("policy: group manifest missing id")
		}
		idLower := strings.ToLower(id)
		if _, exists := seen[idLower]; exists {
			return nil, fmt.Errorf("policy: duplicate group id %q", id)
		}
		seen[idLower] = struct{}{}
		g.ID = id
		g.Members = normalizeList(g.Members)
		result = append(result, g)
	}
	return result, nil
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func parseScope(value string) (Scope, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ScopeDirectory):
		return ScopeDirectory, true
	case string(ScopeTree):
		return ScopeTree, true
	case string(ScopeFile):
		return ScopeFile, true
	default:
		return "", false
	}
}

func parseInheritance(value string) (InheritanceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(InheritanceCascade):
		return InheritanceCascade, true
	case string(InheritanceOverride):
		return InheritanceOverride, true
	case string(InheritanceBreak):
		return InheritanceBreak, true
	default:
		return "", false
	}
}
