package ingesters

import (
	"context"
	"fmt"
	"io"
	"os"
)

// NFSIngester handles reading and writing to/from NFS mounts.
// NFS requires special care for file locking, caching (attribute caching),
// and handling stale file handles (ESTALE).
type NFSIngester struct {
	MountPoint string
}

func NewNFSIngester(mountPoint string) *NFSIngester {
	return &NFSIngester{
		MountPoint: mountPoint,
	}
}

// Read reads a file from the NFS mount.
// Special care: We might need to bypass local caches depending on the NFS version/mount options.
func (i *NFSIngester) Read(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath := fmt.Sprintf("%s/%s", i.MountPoint, path)
	// Add retry logic here for ESTALE if needed in production
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file on NFS: %w", err)
	}
	return f, nil
}

// Write writes data to a file on the NFS mount.
// Special care: Synchronous writes, handling file locks (flock/fcntl), and fsync are critical on NFS.
func (i *NFSIngester) Write(ctx context.Context, path string, data io.Reader) error {
	fullPath := fmt.Sprintf("%s/%s", i.MountPoint, path)
	
	// Use O_SYNC if strict durability is required, or explicitly call Sync() later.
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file on NFS: %w", err)
	}
	defer f.Close()

	// Considerations: Potentially acquire a lock here if concurrent writes are expected.

	if _, err := io.Copy(f, data); err != nil {
		return fmt.Errorf("failed to write data to NFS: %w", err)
	}

	// Important for NFS: force flush to the server to ensure data is actually written
	// and not just residing in local page cache.
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync file on NFS: %w", err)
	}

	return nil
}
