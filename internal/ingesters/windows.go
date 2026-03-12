package ingesters

import (
	"context"
	"fmt"
	"io"
)

// WindowsFileStoreIngester handles backing up and making searchable
// data residing on Windows File Stores (SMB/CIFS).
type WindowsFileStoreIngester struct {
	SharePath string
	Username  string
	Password  string
	Domain    string
}

func NewWindowsFileStoreIngester(sharePath, username, password, domain string) *WindowsFileStoreIngester {
	return &WindowsFileStoreIngester{
		SharePath: sharePath,
		Username:  username,
		Password:  password,
		Domain:    domain,
	}
}

// Backup copies data from the Windows File Store to a local or remote destination.
// This typically requires an SMB client library to mount or read the share.
func (i *WindowsFileStoreIngester) Backup(ctx context.Context, remotePath string, destination io.Writer) error {
	// Implementation would use an SMB library (like github.com/hirochachacha/go-smb2)
	// to connect, authenticate, and read the file stream.
	return fmt.Errorf("SMB backup not fully implemented for path: %s", remotePath)
}

// IndexFile extracts metadata and optionally content from a file on the Windows File Store
// to make it searchable (e.g., sending to Elasticsearch or building a local index).
func (i *WindowsFileStoreIngester) IndexFile(ctx context.Context, remotePath string) error {
	// 1. Connect via SMB.
	// 2. Read file attributes (creation time, modified time, owner ACLs).
	// 3. (Optional) Read file content if it's a known parsable type.
	// 4. Push this structured data to a search backend.
	return fmt.Errorf("file indexing not fully implemented for path: %s", remotePath)
}
