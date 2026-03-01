package v2

type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

var (
	ErrManifestUnknown = ErrorDetail{Code: "MANIFEST_UNKNOWN", Message: "manifest unknown"}
	ErrBlobUnknown     = ErrorDetail{Code: "BLOB_UNKNOWN", Message: "blob unknown"}
	ErrUnauthorized    = ErrorDetail{Code: "UNAUTHORIZED", Message: "authentication required"}
	ErrDenied          = ErrorDetail{Code: "DENIED", Message: "requested access to the resource is denied"}
	ErrUnsupported     = ErrorDetail{Code: "UNSUPPORTED", Message: "The operation is unsupported."}
)
