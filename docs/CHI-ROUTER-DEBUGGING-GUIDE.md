# Chi Router Debugging Guide

## Diagnosing 404 Issues with Overlapping Parameter Patterns

### Symptoms

- Middleware executes successfully (logs show auth checks)
- Handler never gets called (no handler logs)
- Returns 404 Not Found
- Both specific and wildcard routes affected

### Chi Router Matching Algorithm

```
REQUEST: POST /v2/tokenworx/blobs/uploads/

Chi Evaluation Process:
┌─────────────────────────────────────────────────────────┐
│ Step 1: Strip base path (/v2)                           │
│ Remaining: /tokenworx/blobs/uploads/                    │
└─────────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────────┐
│ Step 2: Evaluate registered patterns in ORDER           │
│                                                          │
│ ❌ WRONG ORDER (OLD CODE):                              │
│   1. /{org2}/{org1}/{org}/{namespace}/{repo} (5 params) │
│   2. /{org1}/{org}/{namespace}/{repo} (4 params)        │
│   3. /{org}/{namespace}/{repo} (3 params)               │
│   4. /{namespace}/{repo} (2 params) ← MATCHES!          │
│   5. /{repo} (1 param)                                  │
│                                                          │
│ ✅ CORRECT ORDER (NEW CODE):                            │
│   1. /{org2}/{org1}/{org}/{namespace}/{repo} (5 params) │
│      → Not enough segments, SKIP                        │
│   2. /{org1}/{org}/{namespace}/{repo} (4 params)        │
│      → Not enough segments, SKIP                        │
│   3. /{org}/{namespace}/{repo} (3 params)               │
│      → Not enough segments, SKIP                        │
│   4. /{namespace}/{repo} (2 params)                     │
│      → Not enough segments, SKIP                        │
│   5. /{repo} (1 param) ← MATCHES!                       │
└─────────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────────┐
│ Step 3: Extract parameters                              │
│                                                          │
│ ❌ WRONG: /{namespace}/{repo} match                     │
│   namespace = "tokenworx"                               │
│   repo = "blobs"                                        │
│   Remaining path: /uploads/                             │
│                                                          │
│ ✅ CORRECT: /{repo} match                               │
│   repo = "tokenworx"                                    │
│   Remaining path: /blobs/uploads/                       │
└─────────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────────┐
│ Step 4: Match sub-routes                                │
│                                                          │
│ ❌ WRONG remaining: /uploads/                           │
│   Post("/blobs/uploads/") → NO MATCH → 404              │
│                                                          │
│ ✅ CORRECT remaining: /blobs/uploads/                   │
│   Post("/blobs/uploads/") → MATCH → Handler called!     │
└─────────────────────────────────────────────────────────┘
```

## Chi Router Behavior Analysis

### Why Chi Matches More-Parameter Patterns First

Chi uses a **greedy tree-based routing algorithm**:

1. **Segment-by-segment traversal**: Splits path by `/`
2. **Priority order**:
   - Literal segments (highest priority)
   - Parameter segments `{param}`
   - Wildcard segments `*` (lowest priority)

3. **Parameter counting**: When multiple parameter patterns match, chi prefers the one with MORE parameters

### Example Breakdown

#### Scenario 1: Single-Level Repository

```
Request: /v2/alpine/tags/list

Chi evaluates:
- /{org2}/{org1}/{org}/{namespace}/{repo}/tags/list
  Requires 5 segments before /tags/list → FAIL

- /{org1}/{org}/{namespace}/{repo}/tags/list
  Requires 4 segments before /tags/list → FAIL

- /{org}/{namespace}/{repo}/tags/list
  Requires 3 segments before /tags/list → FAIL

- /{namespace}/{repo}/tags/list
  Requires 2 segments before /tags/list → FAIL

- /{repo}/tags/list
  Requires 1 segment before /tags/list → MATCH!
  repo = "alpine"
```

#### Scenario 2: Two-Level Repository

```
Request: /v2/web3dns/aiserve-farm/tags/list

Chi evaluates:
- /{org2}/{org1}/{org}/{namespace}/{repo}/tags/list
  Requires 5 segments → FAIL

- /{org1}/{org}/{namespace}/{repo}/tags/list
  Requires 4 segments → FAIL

- /{org}/{namespace}/{repo}/tags/list
  Requires 3 segments → FAIL

- /{namespace}/{repo}/tags/list
  Requires 2 segments → MATCH!
  namespace = "web3dns"
  repo = "aiserve-farm"
```

## Debugging Checklist

When you encounter 404 errors with chi router:

### 1. Check Middleware Execution
```go
log.Printf("[MIDDLEWARE] Request: %s %s", r.Method, r.URL.Path)
```

If middleware runs but handler doesn't → **Route pattern mismatch**

### 2. Log Chi Route Matching
```go
// Add to each handler
log.Printf("[HANDLER] Called: %s", r.URL.Path)
```

If no handler logs → **Sub-route matching failed**

### 3. Inspect URL Parameters
```go
// In middleware or handler
log.Printf("[DEBUG] Chi params: %+v", chi.RouteContext(r.Context()).URLParams)
```

If parameters are wrong (e.g., `repo="blobs"`) → **Pattern matched incorrectly**

### 4. Verify Route Registration Order
```go
// Print during router setup
log.Printf("[ROUTER] Registering: %s", pattern)
```

If less-specific patterns register first → **Reorder to most-specific first**

## Common Chi Router Pitfalls

### Pitfall 1: Nested Route() with Overlapping Patterns
```go
// ❌ DON'T DO THIS
api.Route("/{repo}", setupHandlers)
api.Route("/{namespace}/{repo}", setupHandlers)
// Chi will match /{namespace}/{repo} for single-level paths!
```

### Pitfall 2: Using Wildcard `*` in Middle of Pattern
```go
// ❌ INVALID
api.Route("/*/tags/list", handler)
// Chi requires * to be at the END of the pattern
```

### Pitfall 3: Expecting First-Match Behavior
```go
// ❌ WRONG ASSUMPTION
// "Chi will match the first pattern I register"
api.Get("/{repo}/blobs/{digest}", handler1)
api.Get("/{namespace}/{repo}/blobs/{digest}", handler2)
// Chi matches based on parameter COUNT, not registration order!
```

## Best Practices

### 1. Register Most-Specific Routes First
```go
// ✅ CORRECT ORDER
api.Get("/{org}/{namespace}/{repo}/tags/list", handler)  // 3 params
api.Get("/{namespace}/{repo}/tags/list", handler)        // 2 params
api.Get("/{repo}/tags/list", handler)                    // 1 param
```

### 2. Use Direct Registration, Not Nested Route()
```go
// ❌ AVOID
api.Route("/{repo}", func(r chi.Router) {
    r.Get("/tags/list", handler)
})

// ✅ PREFER
api.Get("/{repo}/tags/list", handler)
```

### 3. Extract Repository Path in Handlers, Not in Router
```go
// ✅ GOOD - Handlers parse URL directly
func (r *Router) getTags(w http.ResponseWriter, req *http.Request) {
    fullRepo, _ := getRepoContext(req)  // Parses URL directly
    // ...
}
```

### 4. Add Comprehensive Tests
```go
func TestRouterMatching(t *testing.T) {
    testCases := []struct{
        path string
        expectStatus int
    }{
        {"/v2/alpine/tags/list", http.StatusUnauthorized},  // Should match, auth fails
        {"/v2/org/app/tags/list", http.StatusUnauthorized}, // Should match, auth fails
    }

    for _, tc := range testCases {
        req := httptest.NewRequest("GET", tc.path, nil)
        w := httptest.NewRecorder()
        mux.ServeHTTP(w, req)

        if w.Code == http.StatusNotFound {
            t.Errorf("404 for %s - router pattern not matching!", tc.path)
        }
    }
}
```

## Alternative Solutions

If chi router limitations are too restrictive, consider:

### Option 1: Gorilla Mux
```go
// Gorilla mux supports regex constraints
r.HandleFunc("/v2/{repo:.+}/tags/list", handler)
```

### Option 2: Custom Middleware Router
```go
// Parse repository path in middleware before chi routing
api.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/v2/") {
            // Extract repo, rewrite path, set context
            fullRepo := extractRepo(r.URL.Path)
            ctx := context.WithValue(r.Context(), "fullRepo", fullRepo)
            r = r.WithContext(ctx)
        }
        next.ServeHTTP(w, r)
    })
})
```

### Option 3: httprouter (Fastest)
```go
// httprouter uses radix tree, supports * at end
router.POST("/v2/*repo/blobs/uploads/", handler)
```

## Summary

Chi router is powerful but has specific behaviors around parameter matching:

1. ✅ **DO**: Register routes from most-specific to least-specific
2. ✅ **DO**: Use direct route registration over nested `Route()`
3. ✅ **DO**: Extract repository paths in handlers, not router patterns
4. ✅ **DO**: Add comprehensive tests for multi-level paths

5. ❌ **DON'T**: Assume first-registered route wins
6. ❌ **DON'T**: Use nested `Route()` with overlapping parameter patterns
7. ❌ **DON'T**: Expect wildcard `*` to work mid-pattern

Understanding chi's matching algorithm prevents subtle 404 bugs and ensures reliable routing for complex API patterns like Docker Registry V2.
