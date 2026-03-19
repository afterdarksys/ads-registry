package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient handles HTTP requests to the registry management API
type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient() *APIClient {
	return &APIClient{
		baseURL: getAPIURL(),
		token:   getAdminToken(),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get performs a GET request
func (c *APIClient) Get(path string) ([]byte, error) {
	return c.request("GET", path, nil)
}

// Post performs a POST request
func (c *APIClient) Post(path string, body interface{}) ([]byte, error) {
	return c.request("POST", path, body)
}

// Put performs a PUT request
func (c *APIClient) Put(path string, body interface{}) ([]byte, error) {
	return c.request("PUT", path, body)
}

// Delete performs a DELETE request
func (c *APIClient) Delete(path string) ([]byte, error) {
	return c.request("DELETE", path, nil)
}

func (c *APIClient) request(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	contentType := "application/json"

	if body != nil {
		// Check if body is a string (for plain text uploads)
		if str, ok := body.(string); ok {
			reqBody = bytes.NewBufferString(str)
			contentType = "text/plain"
		} else {
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			reqBody = bytes.NewBuffer(jsonData)
		}
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
