package scripting

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// MANIFEST GENERATOR
// ============================================================================

// ManifestGenerator generates Kubernetes manifests
type ManifestGenerator struct {
}

// NewManifestGenerator creates a new manifest generator
func NewManifestGenerator() *ManifestGenerator {
	return &ManifestGenerator{}
}

// GenerateDeployment generates a Deployment manifest
func (m *ManifestGenerator) GenerateDeployment(name, namespace, image string, replicas int32) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: %s
        image: %s
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
`, name, namespace, replicas, name, name, name, image)
}

// GenerateService generates a Service manifest
func (m *ManifestGenerator) GenerateService(name, namespace, serviceType string, port int32) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  type: %s
  selector:
    app: %s
  ports:
  - protocol: TCP
    port: %d
    targetPort: 8080
`, name, namespace, serviceType, name, port)
}

// GenerateConfigMap generates a ConfigMap manifest
func (m *ManifestGenerator) GenerateConfigMap(name, namespace string, data map[string]string) string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
`, name, namespace)

	for key, value := range data {
		manifest += fmt.Sprintf("  %s: %q\n", key, value)
	}

	return manifest
}

// GenerateSecret generates a Secret manifest
func (m *ManifestGenerator) GenerateSecret(name, namespace string, data map[string]string) string {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
`, name, namespace)

	// Base64 encode secret values
	for key, value := range data {
		encoded := base64.StdEncoding.EncodeToString([]byte(value))
		manifest += fmt.Sprintf("  %s: %s\n", key, encoded)
	}

	return manifest
}

// GenerateImagePullSecret generates an ImagePullSecret manifest
func (m *ManifestGenerator) GenerateImagePullSecret(name, namespace, registry, username, password string) string {
	// Create dockerconfigjson
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			registry: map[string]string{
				"username": username,
				"password": password,
				"auth":     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
			},
		},
	}

	dockerConfigJSON, _ := json.Marshal(dockerConfig)
	encodedConfig := base64.StdEncoding.EncodeToString(dockerConfigJSON)

	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: %s
`, name, namespace, encodedConfig)
}

// GenerateServiceAccount generates a ServiceAccount manifest
func (m *ManifestGenerator) GenerateServiceAccount(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
`, name, namespace)
}

// GenerateNamespace generates a Namespace manifest
func (m *ManifestGenerator) GenerateNamespace(name string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, name)
}

// ============================================================================
// POLICY GENERATOR
// ============================================================================

// PolicyGenerator generates Kubernetes policies
type PolicyGenerator struct {
}

// NewPolicyGenerator creates a new policy generator
func NewPolicyGenerator() *PolicyGenerator {
	return &PolicyGenerator{}
}

// GenerateNetworkPolicy generates a NetworkPolicy manifest
func (p *PolicyGenerator) GenerateNetworkPolicy(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector:
    matchLabels:
      app: %s
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: frontend
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: database
    ports:
    - protocol: TCP
      port: 5432
`, name, namespace, name)
}

// GeneratePodSecurityPolicy generates a PodSecurityPolicy manifest
func (p *PolicyGenerator) GeneratePodSecurityPolicy(name string) string {
	return fmt.Sprintf(`apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: %s
spec:
  privileged: false
  allowPrivilegeEscalation: false
  requiredDropCapabilities:
  - ALL
  volumes:
  - configMap
  - emptyDir
  - projected
  - secret
  - downwardAPI
  - persistentVolumeClaim
  hostNetwork: false
  hostIPC: false
  hostPID: false
  runAsUser:
    rule: MustRunAsNonRoot
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  fsGroup:
    rule: RunAsAny
  readOnlyRootFilesystem: false
`, name)
}

// ============================================================================
// DOCKER COMPOSE CONVERTER
// ============================================================================

// ComposeConverter converts docker-compose files to Kubernetes manifests
type ComposeConverter struct {
	manifestGen *ManifestGenerator
	policyGen   *PolicyGenerator
}

// NewComposeConverter creates a new compose converter
func NewComposeConverter() *ComposeConverter {
	return &ComposeConverter{
		manifestGen: NewManifestGenerator(),
		policyGen:   NewPolicyGenerator(),
	}
}

// ConvertToK8s converts a docker-compose file to Kubernetes manifests
func (c *ComposeConverter) ConvertToK8s(composePath string, isK3s bool) (string, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return "", fmt.Errorf("failed to read compose file: %w", err)
	}

	var compose struct {
		Services map[string]struct {
			Image string   `yaml:"image"`
			Ports []string `yaml:"ports"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal(data, &compose); err != nil {
		return "", fmt.Errorf("failed to parse compose file: %w", err)
	}

	var output string
	for name, svc := range compose.Services {
		output += c.manifestGen.GenerateDeployment(name, "default", svc.Image, 1) + "---\n"
		
		if len(svc.Ports) > 0 {
			port := int32(80)
			parts := strings.Split(svc.Ports[0], ":")
			if len(parts) > 0 {
				if p, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
					port = int32(p)
				}
			}
			
			output += c.manifestGen.GenerateService(name, "default", "ClusterIP", port) + "---\n"
			
			if isK3s {
				output += fmt.Sprintf(`# K3s-specific: Traefik IngressRoute
apiVersion: traefik.containo.us/v1alpha1
kind: IngressRoute
metadata:
  name: %s
  namespace: default
spec:
  entryPoints:
  - web
  routes:
  - match: Host(%s%s.local%s)
    kind: Rule
    services:
    - name: %s
      port: %d
---
`, name, "`", name, "`", name, port)
			}
		}
	}

	return output, nil
}
