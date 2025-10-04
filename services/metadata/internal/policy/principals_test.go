package policy

import "testing"

func TestParsePrincipalManifestUsers(t *testing.T) {
	json := []byte(`{
	  "scope": "directory",
	  "inheritance": "break",
	  "users": [
	    {"id": " alice ", "groups": ["admins", ""], "attributes": {"title": "Lead"}}
	  ]
	}`)
	set, overrides, err := ParsePrincipalManifest(json, TypeUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(set.Users) != 1 {
		t.Fatalf("expected one user, got %d", len(set.Users))
	}
	if set.Users[0].ID != "alice" {
		t.Fatalf("expected id alice, got %s", set.Users[0].ID)
	}
	if len(set.Users[0].Groups) != 1 || set.Users[0].Groups[0] != "admins" {
		t.Fatalf("expected normalized groups, got %#v", set.Users[0].Groups)
	}
	if overrides.Scope == nil || *overrides.Scope != ScopeDirectory {
		t.Fatalf("expected scope override directory")
	}
	if overrides.Inheritance == nil || *overrides.Inheritance != InheritanceBreak {
		t.Fatalf("expected inheritance break")
	}
}

func TestParsePrincipalManifestGroups(t *testing.T) {
	json := []byte(`{"groups": [{"id": "team", "members": [" alice ", "bob"]}]}`)
	set, overrides, err := ParsePrincipalManifest(json, TypeGroup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overrides.Scope != nil || overrides.Inheritance != nil {
		t.Fatalf("expected no overrides")
	}
	if len(set.Groups) != 1 {
		t.Fatalf("expected one group, got %d", len(set.Groups))
	}
	group := set.Groups[0]
	if group.ID != "team" {
		t.Fatalf("unexpected group id %s", group.ID)
	}
	if len(group.Members) != 2 || group.Members[0] != "alice" {
		t.Fatalf("expected normalized members, got %#v", group.Members)
	}
}

func TestParsePrincipalManifestValidation(t *testing.T) {
	_, _, err := ParsePrincipalManifest([]byte(`{"users": [{"id": ""}]}`), TypeUser)
	if err == nil {
		t.Fatal("expected validation error for empty user id")
	}

	_, _, err = ParsePrincipalManifest([]byte(`{"groups": [{"id": "admins"}, {"id": "admins"}]}`), TypeGroup)
	if err == nil {
		t.Fatal("expected duplicate group error")
	}

	_, _, err = ParsePrincipalManifest([]byte(`{}`), TypeWebhook)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
}
