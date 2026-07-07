package storage

import (
	"path/filepath"
	"testing"
)

func TestSafeJoin_BlocksTraversal(t *testing.T) {
	root := "/srv/registry/blobs"
	escapes := []string{
		"../../../etc/passwd",
		"../../etc/cron.d/backdoor",
		"npm/default/../../../../etc/cron.d/x",
		"/etc/passwd",
		"a/b/../../../../../../etc/shadow",
		"../" + string(filepath.Separator) + "escape",
	}
	for _, k := range escapes {
		got, err := SafeJoin(root, k)
		if err == nil && !within(root, got) {
			t.Fatalf("SafeJoin(%q, %q) = %q, escaped root (err=%v)", root, k, got, err)
		}
	}
}

func TestSafeJoin_AllowsLegitimateKeys(t *testing.T) {
	root := "/srv/registry/blobs"
	ok := []string{
		"npm/default/mypkg/mypkg-1.0.0.tgz",
		"sha256:abcdef",
		"apt/default/pool/main/l/lib/x.deb",
		"go/default/example.com/mod/@v/v1.0.0.zip",
	}
	for _, k := range ok {
		got, err := SafeJoin(root, k)
		if err != nil {
			t.Fatalf("SafeJoin(%q, %q) unexpected error: %v", root, k, err)
		}
		if !within(root, got) {
			t.Fatalf("SafeJoin(%q, %q) = %q, not within root", root, k, got)
		}
	}
}

func within(root, p string) bool {
	rc := filepath.Clean(root)
	return p == rc || len(p) > len(rc) && p[:len(rc)] == rc && p[len(rc)] == filepath.Separator
}
