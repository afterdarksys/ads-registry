package automation

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

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

	// Configure the isolated running thread (no shared memory)
	thread := &starlark.Thread{
		Name:  "registry_automation",
		Print: func(_ *starlark.Thread, msg string) { log.Printf("[Starlark] %s", msg) },
	}

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

// httpPostBuiltin provides a native http client to the Starlark runtime
// usage: http_post("https://api.github.com/...", "{\"json\":\"body\"}")
func httpPostBuiltin(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	var body string

	// Ensure Starlark user passed correct type of arguments
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &url, "body?", &body); err != nil {
		return nil, err
	}

	log.Printf("[Starlark Webhook] POST: %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("http request creation failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http post failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Return a Tuple containing (status_code, response_body)
	return starlark.Tuple{
		starlark.MakeInt(resp.StatusCode),
		starlark.String(string(respBody)),
	}, nil
}
