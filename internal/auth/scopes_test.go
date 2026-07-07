package auth

import "testing"

func TestScopesToAccess_NonAdminIsNotWildcard(t *testing.T) {
	access := ScopesToAccess([]string{"repository:myorg/*:pull,push"})
	if len(access) != 1 {
		t.Fatalf("expected 1 access entry, got %d", len(access))
	}
	e := access[0]
	if e.Name == "*" {
		t.Fatalf("non-admin scope must not produce wildcard name; got %+v", e)
	}
	if e.Type != "repository" || e.Name != "myorg/*" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if len(e.Actions) != 2 || e.Actions[0] != "pull" || e.Actions[1] != "push" {
		t.Fatalf("unexpected actions: %v", e.Actions)
	}
}

func TestScopesToAccess_AdminWildcard(t *testing.T) {
	access := ScopesToAccess([]string{"*"})
	if len(access) != 1 || access[0].Name != "*" || access[0].Actions[0] != "*" {
		t.Fatalf("admin scope should map to repository:*:*, got %+v", access)
	}
}

func TestScopesToAccess_BareRepoDefaultsToPullOnly(t *testing.T) {
	access := ScopesToAccess([]string{"repository:foo/bar"})
	if len(access) != 1 || len(access[0].Actions) != 1 || access[0].Actions[0] != "pull" {
		t.Fatalf("bare scope should default to pull-only, got %+v", access)
	}
}

func TestScopesToAccess_MalformedScopeGrantsNothing(t *testing.T) {
	if access := ScopesToAccess([]string{"garbage"}); len(access) != 0 {
		t.Fatalf("malformed scope must grant no access, got %+v", access)
	}
}
