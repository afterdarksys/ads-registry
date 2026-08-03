package storage

import "path"

// BlobPath returns the canonical repository-local key for a registry blob.
// Blob metadata is global by digest, but availability is repository-specific.
func BlobPath(repository, digest string) string {
	return path.Join(repository, digest)
}

// UploadPath returns the canonical key for an in-progress registry upload.
func UploadPath(repository, uuid string) string {
	return path.Join(repository, "uploads", uuid)
}

// RepositoryPath combines separately supplied namespace and repository values
// without duplicating the namespace when repository is already fully qualified.
func RepositoryPath(namespace, repository string) string {
	if namespace == "" || repository == namespace || len(repository) > len(namespace) && repository[:len(namespace)+1] == namespace+"/" {
		return path.Clean(repository)
	}
	return path.Join(namespace, repository)
}
