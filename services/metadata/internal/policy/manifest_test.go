package policy

import "testing"

func TestTypeFromFilename(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		expectOK   bool
		expectType Type
	}{
		{"rego lowercase", ".rego", true, TypeRego},
		{"rego uppercase", ".ReGo", true, TypeRego},
		{"jsonschema", ".jsonschema", true, TypeJSONSchema},
		{"workflow", ".workflow", true, TypeWorkflow},
		{"webhook", ".webhook", true, TypeWebhook},
		{"user", ".user", true, TypeUser},
		{"group", ".group", true, TypeGroup},
		{"events", ".events", true, TypeEvents},
		{"non policy", "document.txt", false, ""},
		{"nested path", "foo/.rego", true, TypeRego},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typ, ok := TypeFromFilename(tc.filename)
			if ok != tc.expectOK {
				t.Fatalf("expected ok=%v, got %v", tc.expectOK, ok)
			}
			if ok && typ != tc.expectType {
				t.Fatalf("expected type %q, got %q", tc.expectType, typ)
			}
		})
	}
}

func TestIsSpecialFile(t *testing.T) {
	if !IsSpecialFile(".workflow") {
		t.Fatalf("expected .workflow to be special")
	}
	if !IsSpecialFile(".events") {
		t.Fatalf("expected .events to be special")
	}
	if IsSpecialFile("notes.txt") {
		t.Fatalf("expected notes.txt to not be special")
	}
}

func TestParseType(t *testing.T) {
	if _, ok := ParseType("unknown"); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if typ, ok := ParseType("ReGo"); !ok || typ != TypeRego {
		t.Fatalf("expected parse rego, got %v %v", typ, ok)
	}
	if typ, ok := ParseType("GROUP"); !ok || typ != TypeGroup {
		t.Fatalf("expected parse group, got %v %v", typ, ok)
	}
	if typ, ok := ParseType("events"); !ok || typ != TypeEvents {
		t.Fatalf("expected parse events, got %v %v", typ, ok)
	}
}

func TestParseScope(t *testing.T) {
	if scope, ok := ParseScope("directory"); !ok || scope != ScopeDirectory {
		t.Fatalf("expected directory scope, got %v %v", scope, ok)
	}
	if scope, ok := ParseScope("TREE"); !ok || scope != ScopeTree {
		t.Fatalf("expected tree scope, got %v %v", scope, ok)
	}
	if _, ok := ParseScope("invalid"); ok {
		t.Fatalf("expected invalid scope to fail")
	}
}

func TestParseInheritanceMode(t *testing.T) {
	if mode, ok := ParseInheritanceMode("cascade"); !ok || mode != InheritanceCascade {
		t.Fatalf("expected cascade mode, got %v %v", mode, ok)
	}
	if mode, ok := ParseInheritanceMode("OVERRIDE"); !ok || mode != InheritanceOverride {
		t.Fatalf("expected override mode, got %v %v", mode, ok)
	}
	if _, ok := ParseInheritanceMode("invalid"); ok {
		t.Fatalf("expected invalid mode to fail")
	}
}

func TestDefaultManifest(t *testing.T) {
	manifest := DefaultManifest(".rego", "/home/.rego", "dir-1", TypeRego)
	if manifest.Type != TypeRego {
		t.Fatalf("expected type %q", TypeRego)
	}
	if manifest.SourcePath != "/home/.rego" {
		t.Fatalf("unexpected source path: %s", manifest.SourcePath)
	}
	if manifest.DirectoryID != "dir-1" {
		t.Fatalf("unexpected directory id: %s", manifest.DirectoryID)
	}
	if manifest.Inheritance != InheritanceCascade {
		t.Fatalf("expected default inheritance cascade")
	}
	if manifest.Scope != ScopeTree {
		t.Fatalf("expected default scope tree")
	}
}

func TestAdminOnly(t *testing.T) {
	if !DefaultManifest(".rego", ".rego", "dir", TypeRego).AdminOnly() {
		t.Fatalf("policy files must be admin only")
	}
}

func TestWithInheritance(t *testing.T) {
	m := DefaultManifest(".rego", ".rego", "dir", TypeRego)
	clone := m.WithInheritance(InheritanceBreak)
	if clone.Inheritance != InheritanceBreak {
		t.Fatalf("expected inheritance override")
	}
	if m.Inheritance != InheritanceCascade {
		t.Fatalf("original manifest mutated")
	}
}
