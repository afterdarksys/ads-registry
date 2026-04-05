package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// EnterpriseInterceptor represents middleware that can parse and rewrite
// request/response bodies and validate payload schemas to enforce enterprise compliance.
type EnterpriseInterceptor struct {
}

// NewEnterpriseInterceptor creates a new interception middleware
func NewEnterpriseInterceptor() *EnterpriseInterceptor {
	return &EnterpriseInterceptor{}
}

// Middleware executes the payload rewriting and schema validation layer
func (ei *EnterpriseInterceptor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		
		// =========================================================================
		// Point 5: Payload Specification Validation 
		// =========================================================================
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				bodyBytes, err := io.ReadAll(r.Body)
				if err == nil && len(bodyBytes) > 0 {
					// Verify compliance with internal JSON Schema specifications
					// For example, protecting against restricted internal fields bypassing the API
					var payload map[string]interface{}
					if err := json.Unmarshal(bodyBytes, &payload); err == nil {
						if _, hasRestricted := payload["$restricted_internal_token"]; hasRestricted {
							http.Error(w, `{"errors":[{"code":"POLICY_VIOLATION","message":"Restricted payload keys detected inside request specification"}]}`, http.StatusBadRequest)
							return
						}
					}
					// Restore body for downstream routers
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			}
		}

		// =========================================================================
		// Point 4: Custom Enterprise JSON/XML Body Injection 
		// =========================================================================
		// We use a custom ResponseWriter to capture the downstream output in a buffer
		rec := &responseRecorder{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK, // default if WriteHeader is never called
		}
		
		next.ServeHTTP(rec, r)
		
		// If the response is JSON, we can parse it and inject custom payload items
		if strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
			var respMap map[string]interface{}
			if err := json.Unmarshal(rec.body.Bytes(), &respMap); err == nil {
				
				// Inject our dynamic Enterprise body configurations
				reqID := r.Header.Get("X-Request-ID")
				if reqID != "" {
					respMap["_enterprise_audit_id"] = reqID
				}
				respMap["_policy_enforced"] = true
				
				modifiedOutput, err := json.Marshal(respMap)
				if err == nil {
					rec.Header().Set("Content-Length", strconv.Itoa(len(modifiedOutput)))
					w.WriteHeader(rec.statusCode)
					w.Write(modifiedOutput)
					return
				}
			}
		}

		// Fallback for non-JSON responses or if unmarshaling failed
		w.WriteHeader(rec.statusCode)
		w.Write(rec.body.Bytes())
	})
}

// responseRecorder captures the HTTP response output to allow rewriting later
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) Header() http.Header {
	return r.ResponseWriter.Header()
}
