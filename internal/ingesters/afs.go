package ingesters

import (
	"context"
	"fmt"
	"io"
	"os"
)

// AFSIngester handles reading, writing, and backing up data to AFS (Andrew File System) volumes.
type AFSIngester struct {
	MountPoint string
}

func NewAFSIngester(mountPoint string) *AFSIngester {
	return &AFSIngester{
		MountPoint: mountPoint,
	}
}

// Write writes data to a file on the AFS volume.
// AFS has a specific cache manager and file semantics (e.g., changes are typically visible upon close).
func (i *AFSIngester) Write(ctx context.Context, path string, data io.Reader) error {
	fullPath := fmt.Sprintf("%s/%s", i.MountPoint, path)

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file on AFS: %w", err)
	}

	if _, err := io.Copy(f, data); err != nil {
		f.Close() // Ensure we close on error
		return fmt.Errorf("failed to write data to AFS: %w", err)
	}

	// In AFS, closing the file usually triggers the actual write to the file server.
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close and flush file to AFS server: %w", err)
	}

	return nil
}

// Backup backs up the specified AFS volume.
// This might involve interacting with AFS-specific command-line tools (like `vos backup`)
// or copying data locally before moving it to a backup tier.
func (i *AFSIngester) BackupVolume(ctx context.Context, volumeName string, destinationPath string) error {
	// Placeholder for AFS volume backup logic.
	// Typically, you might run `vos backup <volume>` and then dump it `vos dump <volume>.backup > file`.
	// For now, this is a simplified stub.
	return fmt.Errorf("AFS volume backup not fully implemented yet for volume: %s", volumeName)
}
