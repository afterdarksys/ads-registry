package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	packageFile    string
	packageVersion string
	packageName    string
	metadata       string
)

var publishCmd = &cobra.Command{
	Use:   "publish [package-file]",
	Short: "Publish an artifact to the registry",
	Long: `Publish a package to the universal artifact registry.

Supports all artifact formats:
  npm:       .tgz tarball
  pypi:      .whl wheel or .tar.gz sdist
  helm:      .tgz chart
  go:        .zip module
  apt:       .deb package
  composer:  .zip archive
  cocoapods: .tar.gz or .zip
  brew:      .tar.gz bottle

Examples:
  # Publish npm package
  artifactadm publish --format npm mypackage-1.0.0.tgz

  # Publish PyPI package
  artifactadm publish --format pypi dist/mypackage-1.0.0-py3-none-any.whl

  # Publish Helm chart
  artifactadm publish --format helm mychart-1.0.0.tgz

  # Publish with custom metadata
  artifactadm publish --format npm mypackage.tgz --metadata '{"author":"John Doe"}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		packageFile = args[0]
		runPublish()
	},
}

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringVar(&packageVersion, "version", "", "Package version (auto-detected if not provided)")
	publishCmd.Flags().StringVar(&packageName, "name", "", "Package name (auto-detected if not provided)")
	publishCmd.Flags().StringVar(&metadata, "metadata", "", "Additional metadata as JSON")
}

func runPublish() {
	regURL := getRegistryURL()
	token := getAuthToken()
	fmt := getFormat()
	ns := getNamespace()

	if verbose {
		fmt.Printf("Publishing %s package to %s\n", fmt, regURL)
		fmt.Printf("Namespace: %s, File: %s\n", ns, packageFile)
	}

	// Validate file exists
	if _, err := os.Stat(packageFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: File not found: %s\n", packageFile)
		os.Exit(1)
	}

	// Route to format-specific publisher
	var err error
	switch fmt {
	case "npm":
		err = publishNPM(regURL, token, ns, packageFile)
	case "pypi":
		err = publishPyPI(regURL, token, ns, packageFile)
	case "helm":
		err = publishHelm(regURL, token, ns, packageFile)
	case "go", "golang":
		err = publishGo(regURL, token, ns, packageFile)
	case "apt":
		err = publishAPT(regURL, token, ns, packageFile)
	case "composer":
		err = publishComposer(regURL, token, ns, packageFile)
	case "cocoapods":
		err = publishCocoaPods(regURL, token, ns, packageFile)
	case "brew", "homebrew":
		err = publishBrew(regURL, token, ns, packageFile)
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported format: %s\n", fmt)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error publishing package: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully published %s\n", filepath.Base(packageFile))
}

func publishNPM(regURL, token, namespace, file string) error {
	// NPM uses PUT with full package.json embedded
	// For simplicity, we'll use a multipart upload
	url := fmt.Sprintf("%s/repository/npm/%s", regURL, extractPackageName(file))

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	// Create a minimal npm publish payload
	payload := map[string]interface{}{
		"_id":      extractPackageName(file),
		"name":     extractPackageName(file),
		"versions": map[string]interface{}{},
		"_attachments": map[string]interface{}{
			filepath.Base(file): map[string]interface{}{
				"content_type": "application/octet-stream",
				"data":         fileData,
			},
		},
	}

	return sendJSON(url, token, "PUT", payload)
}

func publishPyPI(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/pypi/", regURL)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file
	part, err := writer.CreateFormFile("content", filepath.Base(file))
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	part.Write(fileData)

	// Add metadata fields
	if packageName != "" {
		writer.WriteField("name", packageName)
	}
	if packageVersion != "" {
		writer.WriteField("version", packageVersion)
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishHelm(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/helm/api/charts", regURL)

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(fileData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishGo(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/go/upload", regURL)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add zip file
	part, err := writer.CreateFormFile("zip", filepath.Base(file))
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	part.Write(fileData)

	// Add module and version
	if packageName != "" {
		writer.WriteField("module", packageName)
	}
	if packageVersion != "" {
		writer.WriteField("version", packageVersion)
	}

	// Add go.mod file (if exists alongside)
	modFile := strings.TrimSuffix(file, ".zip") + ".mod"
	if modData, err := os.ReadFile(modFile); err == nil {
		modPart, _ := writer.CreateFormFile("mod", filepath.Base(modFile))
		modPart.Write(modData)
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishAPT(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/apt/upload", regURL)

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(fileData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/vnd.debian.binary-package")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishComposer(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/composer/upload", regURL)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("package", filepath.Base(file))
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	part.Write(fileData)

	if packageVersion != "" {
		writer.WriteField("version", packageVersion)
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishCocoaPods(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/cocoapods/api/v1/pods", regURL)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Require podspec
	if metadata == "" {
		return fmt.Errorf("CocoaPods requires --metadata with podspec JSON")
	}

	writer.WriteField("podspec", metadata)

	// Add tarball if provided
	part, err := writer.CreateFormFile("tarball", filepath.Base(file))
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	part.Write(fileData)

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func publishBrew(regURL, token, namespace, file string) error {
	url := fmt.Sprintf("%s/repository/brew/upload", regURL)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("bottle", filepath.Base(file))
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	part.Write(fileData)

	if packageName != "" {
		writer.WriteField("name", packageName)
	}
	if packageVersion != "" {
		writer.WriteField("version", packageVersion)
	}
	if metadata != "" {
		writer.WriteField("json", metadata)
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func sendJSON(url, token, method string, payload interface{}) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func extractPackageName(filename string) string {
	base := filepath.Base(filename)
	// Remove common extensions
	base = strings.TrimSuffix(base, ".tgz")
	base = strings.TrimSuffix(base, ".tar.gz")
	base = strings.TrimSuffix(base, ".whl")
	base = strings.TrimSuffix(base, ".zip")
	base = strings.TrimSuffix(base, ".deb")

	// Remove version pattern (anything after last hyphen)
	parts := strings.Split(base, "-")
	if len(parts) > 1 {
		// Return everything except the last part (usually version)
		return strings.Join(parts[:len(parts)-1], "-")
	}

	return base
}
