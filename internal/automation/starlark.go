package automation

import (
	"fmt"
	"log"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Engine represents the embedded Starlark interpreter
type Engine struct {
	builtins starlark.StringDict
}

func NewEngine() *Engine {
	// Expose native Go functions perfectly to Starlark
	builtins := starlark.StringDict{
		"http_post": starlark.NewBuiltin("http_post", httpPostBuiltin),
	}

	return &Engine{
		builtins: builtins,
	}
}

// ExecuteEvent runs a Starlark script with the provided event metadata (like an image push)
func (e *Engine) ExecuteEvent(scriptPath string, eventName string, eventData map[string]string) error {
	// Construct an event dictionary for Starlark
	eventDict := starlark.NewDict(len(eventData))
	for k, v := range eventData {
		eventDict.SetKey(starlark.String(k), starlark.String(v))
	}

	// Create a structured event object (simulating JSON parsing in Python)
	eventStruct := starlarkstruct.FromStringDict(
		starlark.String("Event"),
		starlark.StringDict{
			"type": starlark.String(eventName),
			"data": eventDict,
		},
	)

	// Configure the isolated running thread (no shared memory).
	// Cancel the thread after execTimeout to prevent runaway scripts.
	thread := &starlark.Thread{
		Name:  "registry_automation",
		Print: func(_ *starlark.Thread, msg string) { log.Printf("[Starlark] %s", msg) },
	}
	timer := time.AfterFunc(execTimeout, func() {
		thread.Cancel("execution timeout exceeded")
	})
	defer timer.Stop()

	// Load and compile the target starlark .star file
	globals, err := starlark.ExecFile(thread, scriptPath, nil, e.builtins)
	if err != nil {
		return fmt.Errorf("failed to execute starlark script %s: %w", scriptPath, err)
	}

	// Look for a mapped handler function based on the event name (e.g. "on_push" matching def on_push(event):)
	handlerName := fmt.Sprintf("on_%s", eventName)
	if handler, ok := globals[handlerName]; ok {
		if fn, ok := handler.(starlark.Callable); ok {
			// Invoke the parsed Starlark Python function natively from Go
			log.Printf("[Starlark] Invoking %s handler from %s", handlerName, scriptPath)
			_, err := starlark.Call(thread, fn, starlark.Tuple{eventStruct}, nil)
			if err != nil {
				return fmt.Errorf("failed to call starlark handler %s: %w", handlerName, err)
			}
		}
	} else {
		log.Printf("[Starlark] Warning: Hook map '%s' not found in script %s", handlerName, scriptPath)
	}

	return nil
}

// EvaluateSyncPolicy evaluates a Starlark python script returning a bool boolean to control registry-to-registry syncing
func (e *Engine) EvaluateSyncPolicy(scriptPath string, eventData map[string]string) (bool, error) {
	eventDict := starlark.NewDict(len(eventData))
	for k, v := range eventData {
		eventDict.SetKey(starlark.String(k), starlark.String(v))
	}

	eventStruct := starlarkstruct.FromStringDict(
		starlark.String("Event"),
		starlark.StringDict{
			"type": starlark.String("sync_attempt"),
			"data": eventDict,
		},
	)

	thread := &starlark.Thread{
		Name:  "registry_sync_policer",
		Print: func(_ *starlark.Thread, msg string) { log.Printf("[Starlark Sync] %s", msg) },
	}
	timer := time.AfterFunc(execTimeout, func() {
		thread.Cancel("execution timeout exceeded")
	})
	defer timer.Stop()

	globals, err := starlark.ExecFile(thread, scriptPath, nil, e.builtins)
	if err != nil {
		// If script is missing entirely, we default to block for safety or allow?
		// Usually if file not found, we might want to return true.
		// Let's defer that to the caller by returning the error.
		return false, fmt.Errorf("failed to execute policy script %s: %w", scriptPath, err)
	}

	handlerName := "on_sync_attempt"
	if handler, ok := globals[handlerName]; ok {
		if fn, ok := handler.(starlark.Callable); ok {
			val, err := starlark.Call(thread, fn, starlark.Tuple{eventStruct}, nil)
			if err != nil {
				return false, fmt.Errorf("failed to call starlark handler %s: %w", handlerName, err)
			}
			
			if boolVal, ok := val.(starlark.Bool); ok {
				return bool(boolVal), nil
			}
			return false, fmt.Errorf("on_sync_attempt must return a boolean, got %s", val.Type())
		}
	}

	// Default allow if no policy hook is explicitly defined in a found script
	return true, nil
}

// httpPostBuiltin provides a sandboxed HTTP POST to the Starlark runtime.
// Requests to private/loopback IP ranges are blocked to prevent SSRF attacks.
// usage: http_post("https://api.github.com/...", "{\"json\":\"body\"}")
func httpPostBuiltin(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var rawURL string
	var body string

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &rawURL, "body?", &body); err != nil {
		return nil, err
	}

	log.Printf("[Starlark Webhook] POST: %s", rawURL)

	status, respBody, err := sandboxedPost(rawURL, body)
	if err != nil {
		return nil, err
	}

	// Return a Tuple containing (status_code, response_body)
	return starlark.Tuple{
		starlark.MakeInt(status),
		starlark.String(respBody),
	}, nil
}
