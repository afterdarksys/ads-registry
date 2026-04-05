package middleware

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"
)

type debugRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *debugRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *debugRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// OCIDebugger acts as an advanced traffic interceptor dumping byte flows to local traces
func OCIDebugger() func(next http.Handler) http.Handler {
	_ = os.MkdirAll(filepath.Join("logs", "traces"), 0755)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			
			// Dump the incoming request natively
			reqDump, err := httputil.DumpRequest(r, true)
			var reqData string
			if err != nil {
				reqData = fmt.Sprintf("Error dumping request: %v", err)
			} else {
				reqData = string(reqDump)
			}

			// Capture the response
			rec := &debugRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
				statusCode:     http.StatusOK, // Default if WriteHeader is not called explictly
			}

			next.ServeHTTP(rec, r)

			// Basic Response Header Builder for dumping representation
			respHeaderData := fmt.Sprintf("HTTP/1.1 %d %s\r\n", rec.statusCode, http.StatusText(rec.statusCode))
			for k, v := range rec.Header() {
				respHeaderData += fmt.Sprintf("%s: %s\r\n", k, v[0]) // Simplistic joiner
			}
			respHeaderData += "\r\n"

			// Finalize trace
			duration := time.Since(startTime)
			traceID := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))[:12]
			traceFile := filepath.Join("logs", "traces", fmt.Sprintf("trace_%s.log", traceID))

			traceContent := fmt.Sprintf("=== OCI PROTOCOL TRACE [%s] ===\n", traceID)
			traceContent += fmt.Sprintf("TIMESTAMP : %v\n", startTime)
			traceContent += fmt.Sprintf("DURATION  : %v\n", duration)
			traceContent += fmt.Sprintf("REMOTE_IP : %s\n", r.RemoteAddr)
			traceContent += "=== REQUEST ===\n"
			traceContent += reqData + "\n"
			traceContent += "=== RESPONSE ===\n"
			traceContent += respHeaderData
			
			// Optional: Truncate response body if it is too massive (e.g. huge blobs)
			if rec.body.Len() < 1024*1024*5 { // Limit to 5MB dumps
				traceContent += rec.body.String()
			} else {
				traceContent += fmt.Sprintf("<body truncated: %d bytes>", rec.body.Len())
			}

			if err := os.WriteFile(traceFile, []byte(traceContent), 0644); err != nil {
				log.Printf("[Debugger] Error saving trace file: %v", err)
			}
		})
	}
}
