package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/ryan/ads-registry/internal/db"
	"github.com/ryan/ads-registry/internal/scanner"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Engine is the Starlark scripting engine
type Engine struct {
	predeclared starlark.StringDict
	builtins    *BuiltinFunctions
	k8sHelpers  *K8sHelpers
}

// NewEngine creates a new Starlark scripting engine
func NewEngine() *Engine {
	return NewEngineWithDeps(nil, nil)
}

// NewEngineWithDeps creates a new Starlark scripting engine with optional db and scanner
func NewEngineWithDeps(dbStore db.Store, sc scanner.Scanner) *Engine {
	builtins := NewBuiltinFunctions(dbStore, sc)
	k8sHelpers := NewK8sHelpers()

	predeclared := starlark.StringDict{
		// Registry operations
		"registry": starlarkstruct.FromStringDict(starlark.String("registry"), builtins.RegistryModule()),

		// Image operations
		"image": starlarkstruct.FromStringDict(starlark.String("image"), builtins.ImageModule()),

		// Kubernetes helpers
		"k8s": starlarkstruct.FromStringDict(starlark.String("k8s"), k8sHelpers.Module()),

		// Docker Compose migration
		"compose": starlarkstruct.FromStringDict(starlark.String("compose"), k8sHelpers.ComposeModule()),

		// Utilities
		"scan":   starlarkstruct.FromStringDict(starlark.String("scan"), builtins.ScanModule()),
		"policy": starlarkstruct.FromStringDict(starlark.String("policy"), builtins.PolicyModule()),
	}

	return &Engine{
		predeclared: predeclared,
		builtins:    builtins,
		k8sHelpers:  k8sHelpers,
	}
}

// ExecuteScript executes a Starlark script
func (e *Engine) ExecuteScript(ctx context.Context, script string, filename string) (starlark.StringDict, error) {
	thread := &starlark.Thread{
		Name: "ads-registry",
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Println(msg)
		},
	}

	// Execute the script
	globals, err := starlark.ExecFile(thread, filename, script, e.predeclared)
	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w", err)
	}

	return globals, nil
}

// ExecuteFile executes a Starlark script from a file
func (e *Engine) ExecuteFile(ctx context.Context, path string) (starlark.StringDict, error) {
	thread := &starlark.Thread{
		Name: "ads-registry",
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Println(msg)
		},
	}

	globals, err := starlark.ExecFile(thread, path, nil, e.predeclared)
	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w", err)
	}

	return globals, nil
}

// ============================================================================
// BUILTIN FUNCTIONS
// ============================================================================

// BuiltinFunctions provides registry-specific built-in functions
type BuiltinFunctions struct {
	db      db.Store
	scanner scanner.Scanner
}

// NewBuiltinFunctions creates new builtin functions
func NewBuiltinFunctions(dbStore db.Store, sc scanner.Scanner) *BuiltinFunctions {
	return &BuiltinFunctions{
		db:      dbStore,
		scanner: sc,
	}
}

// RegistryModule provides registry operations
func (b *BuiltinFunctions) RegistryModule() starlark.StringDict {
	return starlark.StringDict{
		"list_images":     starlark.NewBuiltin("list_images", b.listImages),
		"get_image":       starlark.NewBuiltin("get_image", b.getImage),
		"delete_image":    starlark.NewBuiltin("delete_image", b.deleteImage),
		"copy_image":      starlark.NewBuiltin("copy_image", b.copyImage),
		"create_repo":     starlark.NewBuiltin("create_repo", b.createRepo),
		"list_repos":      starlark.NewBuiltin("list_repos", b.listRepos),
		"set_permissions": starlark.NewBuiltin("set_permissions", b.setPermissions),
	}
}

// ImageModule provides image manipulation operations
func (b *BuiltinFunctions) ImageModule() starlark.StringDict {
	return starlark.StringDict{
		"get_manifest": starlark.NewBuiltin("get_manifest", b.getManifest),
		"get_config":   starlark.NewBuiltin("get_config", b.getConfig),
		"get_layers":   starlark.NewBuiltin("get_layers", b.getLayers),
		"get_tags":     starlark.NewBuiltin("get_tags", b.getTags),
		"add_tag":      starlark.NewBuiltin("add_tag", b.addTag),
		"remove_tag":   starlark.NewBuiltin("remove_tag", b.removeTag),
	}
}

// ScanModule provides security scanning operations
func (b *BuiltinFunctions) ScanModule() starlark.StringDict {
	return starlark.StringDict{
		"scan_image":           starlark.NewBuiltin("scan_image", b.scanImage),
		"get_scan_results":     starlark.NewBuiltin("get_scan_results", b.getScanResults),
		"analyze_supply_chain": starlark.NewBuiltin("analyze_supply_chain", b.analyzeSupplyChain),
	}
}

// PolicyModule provides policy operations
func (b *BuiltinFunctions) PolicyModule() starlark.StringDict {
	return starlark.StringDict{
		"create_lifecycle_policy": starlark.NewBuiltin("create_lifecycle_policy", b.createLifecyclePolicy),
		"apply_policy":            starlark.NewBuiltin("apply_policy", b.applyPolicy),
		"list_policies":           starlark.NewBuiltin("list_policies", b.listPolicies),
	}
}

// ============================================================================
// REGISTRY MODULE IMPLEMENTATIONS
// ============================================================================

func (b *BuiltinFunctions) listImages(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if b.db == nil {
		return starlark.NewList([]starlark.Value{}), nil
	}
	ctx := context.Background()
	records, err := b.db.ListManifests(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_images: %w", err)
	}
	items := make([]starlark.Value, 0, len(records))
	for _, rec := range records {
		d := starlark.NewDict(4)
		_ = d.SetKey(starlark.String("namespace"), starlark.String(rec.Namespace))
		_ = d.SetKey(starlark.String("repo"), starlark.String(rec.Repo))
		_ = d.SetKey(starlark.String("reference"), starlark.String(rec.Reference))
		_ = d.SetKey(starlark.String("digest"), starlark.String(rec.Digest))
		items = append(items, d)
	}
	return starlark.NewList(items), nil
}

func (b *BuiltinFunctions) getImage(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, reference string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "reference", &reference); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	mediaType, digest, payload, err := b.db.GetManifest(ctx, repo, reference)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return starlark.None, nil
		}
		return nil, fmt.Errorf("get_image: %w", err)
	}
	d := starlark.NewDict(3)
	_ = d.SetKey(starlark.String("mediaType"), starlark.String(mediaType))
	_ = d.SetKey(starlark.String("digest"), starlark.String(digest))
	_ = d.SetKey(starlark.String("payload"), starlark.String(string(payload)))
	return d, nil
}

func (b *BuiltinFunctions) deleteImage(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, reference string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "reference", &reference); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	if err := b.db.DeleteManifest(ctx, repo, reference); err != nil {
		return nil, fmt.Errorf("delete_image: %w", err)
	}
	return starlark.None, nil
}

func (b *BuiltinFunctions) copyImage(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Not yet implemented — cross-registry copy requires storage access
	return starlark.None, nil
}

func (b *BuiltinFunctions) createRepo(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var namespace, name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "namespace", &namespace, "name", &name); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	// Ignore ErrAlreadyExists for namespace creation
	if err := b.db.CreateNamespace(ctx, namespace); err != nil && !isAlreadyExists(err) {
		return nil, fmt.Errorf("create_repo: create namespace: %w", err)
	}
	if err := b.db.CreateRepository(ctx, namespace, name); err != nil {
		return nil, fmt.Errorf("create_repo: create repository: %w", err)
	}
	return starlark.None, nil
}

func (b *BuiltinFunctions) listRepos(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if b.db == nil {
		return starlark.NewList([]starlark.Value{}), nil
	}
	ctx := context.Background()
	repos, err := b.db.ListRepositories(ctx, 100, "")
	if err != nil {
		return nil, fmt.Errorf("list_repos: %w", err)
	}
	items := make([]starlark.Value, 0, len(repos))
	for _, r := range repos {
		items = append(items, starlark.String(r))
	}
	return starlark.NewList(items), nil
}

func (b *BuiltinFunctions) setPermissions(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Permission management not yet exposed through scripting layer
	return starlark.None, nil
}

// ============================================================================
// IMAGE MODULE IMPLEMENTATIONS
// ============================================================================

func (b *BuiltinFunctions) getManifest(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, reference string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "reference", &reference); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	mediaType, digest, payload, err := b.db.GetManifest(ctx, repo, reference)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return starlark.None, nil
		}
		return nil, fmt.Errorf("get_manifest: %w", err)
	}
	d := starlark.NewDict(3)
	_ = d.SetKey(starlark.String("mediaType"), starlark.String(mediaType))
	_ = d.SetKey(starlark.String("digest"), starlark.String(digest))
	_ = d.SetKey(starlark.String("payload"), starlark.String(string(payload)))
	return d, nil
}

func (b *BuiltinFunctions) getConfig(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Config blob retrieval requires storage access — not yet wired
	return starlark.None, nil
}

func (b *BuiltinFunctions) getLayers(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, reference string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "reference", &reference); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.NewList([]starlark.Value{}), nil
	}
	ctx := context.Background()
	_, _, payload, err := b.db.GetManifest(ctx, repo, reference)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return starlark.NewList([]starlark.Value{}), nil
		}
		return nil, fmt.Errorf("get_layers: %w", err)
	}

	var manifest struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return starlark.NewList([]starlark.Value{}), nil
	}

	items := make([]starlark.Value, 0, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		items = append(items, starlark.String(layer.Digest))
	}
	return starlark.NewList(items), nil
}

func (b *BuiltinFunctions) getTags(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.NewList([]starlark.Value{}), nil
	}
	ctx := context.Background()
	tags, err := b.db.ListTags(ctx, repo, 100, "")
	if err != nil {
		return nil, fmt.Errorf("get_tags: %w", err)
	}
	items := make([]starlark.Value, 0, len(tags))
	for _, t := range tags {
		items = append(items, starlark.String(t))
	}
	return starlark.NewList(items), nil
}

func (b *BuiltinFunctions) addTag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, fromRef, toRef string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "from_ref", &fromRef, "to_ref", &toRef); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	mediaType, digest, payload, err := b.db.GetManifest(ctx, repo, fromRef)
	if err != nil {
		return nil, fmt.Errorf("add_tag: get source manifest: %w", err)
	}
	if err := b.db.PutManifest(ctx, repo, toRef, mediaType, digest, payload); err != nil {
		return nil, fmt.Errorf("add_tag: put target manifest: %w", err)
	}
	return starlark.None, nil
}

func (b *BuiltinFunctions) removeTag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, reference string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "reference", &reference); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	if err := b.db.DeleteManifest(ctx, repo, reference); err != nil {
		return nil, fmt.Errorf("remove_tag: %w", err)
	}
	return starlark.None, nil
}

// ============================================================================
// SCAN MODULE IMPLEMENTATIONS
// ============================================================================

func (b *BuiltinFunctions) scanImage(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if b.scanner == nil {
		return starlark.None, fmt.Errorf("scanner not configured")
	}
	var repo, digest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "repo", &repo, "digest", &digest); err != nil {
		return nil, err
	}
	// Enqueue an async scan job; the scanner worker processes it in the background
	b.scanner.Enqueue("", repo, digest, digest)
	return starlark.None, nil
}

func (b *BuiltinFunctions) getScanResults(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var digest, scannerName string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "digest", &digest, "scanner_name", &scannerName); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	data, err := b.db.GetScanReport(ctx, digest, scannerName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return starlark.None, nil
		}
		return nil, fmt.Errorf("get_scan_results: %w", err)
	}
	return starlark.String(string(data)), nil
}

func (b *BuiltinFunctions) analyzeSupplyChain(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	log.Println("[scripting] supply chain analysis not yet implemented")
	return starlark.None, nil
}

// ============================================================================
// POLICY MODULE IMPLEMENTATIONS
// ============================================================================

func (b *BuiltinFunctions) createLifecyclePolicy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var expression string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "expression", &expression); err != nil {
		return nil, err
	}
	if b.db == nil {
		return starlark.None, nil
	}
	ctx := context.Background()
	if err := b.db.AddPolicy(ctx, expression); err != nil {
		return nil, fmt.Errorf("create_lifecycle_policy: %w", err)
	}
	return starlark.None, nil
}

func (b *BuiltinFunctions) applyPolicy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return b.listPolicies(thread, fn, args, kwargs)
}

func (b *BuiltinFunctions) listPolicies(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if b.db == nil {
		return starlark.NewList([]starlark.Value{}), nil
	}
	ctx := context.Background()
	policies, err := b.db.ListPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_policies: %w", err)
	}
	items := make([]starlark.Value, 0, len(policies))
	for _, p := range policies {
		d := starlark.NewDict(2)
		_ = d.SetKey(starlark.String("id"), starlark.MakeInt(p.ID))
		_ = d.SetKey(starlark.String("expression"), starlark.String(p.Expression))
		items = append(items, d)
	}
	return starlark.NewList(items), nil
}

// ============================================================================
// HELPERS
// ============================================================================

// isAlreadyExists returns true for errors that indicate the resource already
// exists (e.g. duplicate-key errors from the DB layer).
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "already exists") || contains(msg, "duplicate") || contains(msg, "UNIQUE constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
