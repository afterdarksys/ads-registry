package storage

// Threats: defends against path-traversal (CWE-22) in client-controlled storage
// keys — package names, versions, upload filenames, mount digests, and Go module
// paths all flow into storage keys. SafeJoin guarantees the resolved path cannot
// escape the storage root. It does NOT defend against symlinks already present
// inside the root (see local storage O_NOFOLLOW note in CLAUDE.md).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a storage key would resolve outside the root.
var ErrUnsafePath = errors.New("storage: path escapes root directory")

// SafeJoin joins root and key, guaranteeing the result stays within root.
//
// It neutralizes traversal by cleaning key as if it were rooted at "/" (so any
// leading "../" sequences are absorbed) before joining, then verifies the final
// path is contained within root as a belt-and-suspenders check. A key such as
// "../../../etc/passwd" resolves to "<root>/etc/passwd", never outside root.
func SafeJoin(root, key string) (string, error) {
	// filepath.Clean("/" + key) turns "../../x" into "/x" and "/etc/passwd"
	// stays "/etc/passwd"; joining with root then re-anchors it under root.
	cleanKey := filepath.Clean("/" + strings.TrimPrefix(key, "/"))
	full := filepath.Join(root, cleanKey)

	rootClean := filepath.Clean(root)
	if full != rootClean && !strings.HasPrefix(full, rootClean+string(os.PathSeparator)) {
		return "", ErrUnsafePath
	}
	return full, nil
}
