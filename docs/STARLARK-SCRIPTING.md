## Starlark Scripting for ADS Container Registry

**Powerful automation and customization through Python-like scripts**

---

## Table of Contents

1. [Introduction](#introduction)
2. [Why Starlark?](#why-starlark)
3. [Getting Started](#getting-started)
4. [API Reference](#api-reference)
5. [Example Scripts](#example-scripts)
6. [Best Practices](#best-practices)
7. [Advanced Usage](#advanced-usage)

---

## Introduction

The ADS Container Registry includes a powerful Starlark scripting engine that allows you to:

- **Automate registry operations**: Bulk image management, lifecycle policies, cleanup
- **Migrate infrastructure**: Convert Docker Compose to Kubernetes/K3s manifests
- **Generate Kubernetes resources**: Deployments, Services, ConfigMaps, Secrets, Policies
- **Analyze security**: Supply chain gap analysis, vulnerability reporting
- **Customize workflows**: Build tailored automation for your specific needs

Starlark is a **Python-like language** designed for configuration and scripting. It's:
- **Familiar**: If you know Python, you know Starlark
- **Safe**: No infinite loops, no arbitrary code execution
- **Deterministic**: Same script always produces same results
- **Fast**: Compiled and optimized for performance

---

## Why Starlark?

### Used by Industry Leaders

Starlark is used in production by:

- **Google Bazel** - Build system for massive monorepos
- **Buck** - Facebook's build system
- **Copybara** - Google's code migration tool
- **Drone CI** - Configuration for CI/CD pipelines
- **Isopod** - Kubernetes application management

### Advantages Over Alternatives

| Feature | Starlark | Bash | Python | Go templates |
|---------|----------|------|--------|--------------|
| Safe execution | ✅ | ❌ | ❌ | ✅ |
| Easy to learn | ✅ | ❌ | ✅ | ❌ |
| Type safety | ✅ | ❌ | ❌ | ⚠️ |
| Cross-platform | ✅ | ❌ | ✅ | ✅ |
| Registry API access | ✅ | ⚠️ | ⚠️ | ❌ |
| No external deps | ✅ | ✅ | ❌ | ✅ |

---

## Getting Started

### Installation

The Starlark engine is built into the ADS Container Registry. No additional installation required.

### Your First Script

Create `hello.star`:

```python
#!/usr/bin/env ads-registry script

def main():
    print("Hello from ADS Registry!")

    # List all images
    images = registry.list_images()
    print(f"Found {len(images)} images in registry")

    # List all repositories
    repos = registry.list_repos()
    for repo in repos:
        print(f"  - {repo['name']}")

main()
```

Run it:

```bash
ads-registry script hello.star
```

Output:
```
Hello from ADS Registry!
Found 42 images in registry
  - myapp/frontend
  - myapp/backend
  - myapp/database
```

---

## API Reference

### Registry Module (`registry.*`)

Operations on the registry itself.

```python
# List all images
images = registry.list_images()
images = registry.list_images(owner_id=123)  # Filter by owner

# Get specific image
image = registry.get_image("myapp/frontend:latest")
image = registry.get_image(digest="sha256:abc123...")

# Create repository
registry.create_repo(name="myapp/newservice", visibility="private")

# List repositories
repos = registry.list_repos()

# Set permissions
registry.set_permissions(
    repo_id=42,
    user_id=10,
    permission="push"
)

# Delete image
registry.delete_image(digest="sha256:abc123...")

# Copy image
registry.copy_image(
    source="myapp/frontend:v1.0",
    destination="myapp/frontend:v1.0-backup"
)
```

### Image Module (`image.*`)

Operations on specific images.

```python
# Get manifest
manifest = image.get_manifest(digest="sha256:abc123...")

# Get config
config = image.get_config(digest="sha256:abc123...")

# Get layers
layers = image.get_layers(digest="sha256:abc123...")

# Get tags
tags = image.get_tags(digest="sha256:abc123...")

# Add tag
image.add_tag(digest="sha256:abc123...", tag="v2.0")

# Remove tag
image.remove_tag(repo="myapp/frontend", tag="latest")
```

### Scan Module (`scan.*`)

Security scanning operations.

```python
# Scan image for vulnerabilities
scan.scan_image(digest="sha256:abc123...")

# Get scan results
results = scan.get_scan_results(digest="sha256:abc123...")
print(f"Found {results['critical_vulnerabilities']} critical CVEs")

# Analyze supply chain security
analysis = scan.analyze_supply_chain(digest="sha256:abc123...")
print(f"SBOM present: {analysis['sbom']['present']}")
print(f"Signed: {analysis['signatures']['signed']}")
print(f"SLSA Level: {analysis['provenance']['slsa_level']}")
print(f"Security Score: {analysis['overall_score']}/100")
```

### Kubernetes Module (`k8s.*`)

Generate Kubernetes manifests.

```python
# Create Deployment
deployment = k8s.deployment(
    name="myapp",
    namespace="production",
    image="apps.afterdarksys.com:5005/myapp:latest",
    replicas=3
)
print(deployment)  # Prints YAML

# Create Service
service = k8s.service(
    name="myapp",
    namespace="production",
    type="LoadBalancer",
    port=80
)

# Create ConfigMap
config = k8s.config_map(
    name="myapp-config",
    namespace="production",
    data={
        "database_host": "postgres.default.svc.cluster.local",
        "redis_host": "redis.default.svc.cluster.local"
    }
)

# Create Secret
secret = k8s.secret(
    name="myapp-secret",
    namespace="production",
    data={
        "database_password": "supersecret",
        "api_key": "abc123def456"
    }
)

# Create ImagePullSecret
image_pull_secret = k8s.image_pull_secret(
    name="registry-secret",
    namespace="production",
    registry="apps.afterdarksys.com:5005",
    username="admin",
    password="password"
)

# Create NetworkPolicy
network_policy = k8s.network_policy(
    name="myapp-network-policy",
    namespace="production"
)

# Create PodSecurityPolicy
psp = k8s.pod_security_policy(name="restricted")

# Create ServiceAccount
sa = k8s.service_account(
    name="myapp-sa",
    namespace="production"
)

# Create Namespace
ns = k8s.namespace(name="production")
```

### Docker Compose Migration (`compose.*`)

Convert Docker Compose to Kubernetes.

```python
# Convert docker-compose.yml to Kubernetes manifests
manifests = compose.to_k8s(
    compose_file="docker-compose.yml",
    output="k8s-manifests"
)
print(manifests)

# Convert to K3s (uses Traefik IngressRoute instead of Ingress)
manifests = compose.to_k3s(
    compose_file="docker-compose.yml",
    output="k3s-manifests"
)

# Parse docker-compose file
services = compose.parse("docker-compose.yml")
for service_name in services:
    print(f"{service_name}: {services[service_name]['image']}")

# Validate docker-compose file
is_valid = compose.validate("docker-compose.yml")
```

### Policy Module (`policy.*`)

Lifecycle and governance policies.

```python
# Create lifecycle policy
policy.create_lifecycle_policy(
    name="cleanup-old-images",
    rules=[
        {
            "type": "max_age",
            "value": 90,
            "action": "delete"
        },
        {
            "type": "untagged",
            "action": "delete"
        },
        {
            "type": "critical_cve",
            "action": "quarantine"
        }
    ]
)

# Apply policy to repository
policy.apply_policy(
    policy_name="cleanup-old-images",
    repo_name="myapp/frontend"
)

# List policies
policies = policy.list_policies()
```

---

## Example Scripts

### 1. Docker Compose to Kubernetes Migration

See: `scripts/examples/migrate-compose-to-k8s.star`

```python
# Automatically converts docker-compose.yml to full K8s setup
compose_file = "docker-compose.yml"
manifests = compose.to_k8s(compose_file=compose_file)

# Generates:
# - Namespace
# - ImagePullSecrets
# - Deployments (one per service)
# - Services (for exposed ports)
# - ConfigMaps (from environment variables)
# - NetworkPolicies
```

### 2. Automated Image Cleanup

See: `scripts/examples/cleanup-old-images.star`

```python
# Clean up old, untagged, or vulnerable images
images = registry.list_images()

for image in images:
    # Delete if untagged
    if len(image.get_tags(image["digest"])) == 0:
        registry.delete_image(image["digest"])

    # Delete if too old
    if image["age_days"] > 90:
        registry.delete_image(image["digest"])

    # Delete if critical CVEs
    scan_results = scan.get_scan_results(image["digest"])
    if scan_results["critical_vulnerabilities"] > 0:
        registry.delete_image(image["digest"])
```

### 3. Supply Chain Security Report

See: `scripts/examples/supply-chain-report.star`

```python
# Generate comprehensive supply chain security report
images = registry.list_images(owner_id=user_id)

for image in images:
    analysis = scan.analyze_supply_chain(image["digest"])

    print(f"Image: {image['name']}")
    print(f"  SBOM: {'✓' if analysis['sbom']['present'] else '✗'}")
    print(f"  Signed: {'✓' if analysis['signatures']['signed'] else '✗'}")
    print(f"  SLSA Level: {analysis['provenance']['slsa_level']}")
    print(f"  Security Score: {analysis['overall_score']}/100")

    # Show gaps
    for gap in analysis['gaps']:
        print(f"  [GAP] {gap['description']}")
        print(f"        Remediation: {gap['remediation']}")
```

### 4. Bulk Image Tagging

```python
#!/usr/bin/env ads-registry script

# Tag all images from main branch as 'production'
images = registry.list_images()

for image in images:
    tags = image.get_tags(image["digest"])

    if "main" in tags:
        # Also tag as 'production'
        image.add_tag(
            digest=image["digest"],
            tag="production"
        )
        print(f"Tagged {image['name']} as production")
```

### 5. Generate Development Environment

```python
#!/usr/bin/env ads-registry script

# Generate complete K8s dev environment for a microservices app
services = ["frontend", "backend", "database", "redis"]
namespace = "dev"

# Create namespace
print(k8s.namespace(name=namespace))

# Create ImagePullSecret
print(k8s.image_pull_secret(
    name="registry-secret",
    namespace=namespace,
    registry="apps.afterdarksys.com:5005",
    username="dev-user",
    password="dev-password"
))

# Create resources for each service
for service in services:
    image = f"apps.afterdarksys.com:5005/myapp/{service}:dev"

    print(k8s.deployment(
        name=service,
        namespace=namespace,
        image=image,
        replicas=1
    ))

    print(k8s.service(
        name=service,
        namespace=namespace,
        type="ClusterIP",
        port=8080
    ))

    print(k8s.network_policy(
        name=f"{service}-network-policy",
        namespace=namespace
    ))
```

---

## Best Practices

### 1. Use Functions for Reusability

```python
def create_microservice(name, namespace, image, port):
    """Create a complete microservice deployment"""

    # Deployment
    print(k8s.deployment(
        name=name,
        namespace=namespace,
        image=image,
        replicas=3
    ))

    # Service
    print(k8s.service(
        name=name,
        namespace=namespace,
        type="ClusterIP",
        port=port
    ))

    # NetworkPolicy
    print(k8s.network_policy(
        name=f"{name}-network-policy",
        namespace=namespace
    ))

# Use it
create_microservice("frontend", "prod", "myapp/frontend:v1.0", 80)
create_microservice("backend", "prod", "myapp/backend:v1.0", 8080)
```

### 2. Configuration at the Top

```python
# Configuration
NAMESPACE = "production"
REGISTRY = "apps.afterdarksys.com:5005"
REPLICAS = 3
DRY_RUN = True

# Script uses these constants
# Makes it easy to change behavior without editing code
```

### 3. Error Handling

```python
def safe_delete(digest):
    """Safely delete an image with error handling"""
    try:
        registry.delete_image(digest)
        return True
    except Exception as e:
        print(f"Failed to delete {digest}: {e}")
        return False

# Use it
for image in old_images:
    if safe_delete(image["digest"]):
        print(f"✓ Deleted {image['name']}")
```

### 4. Dry Run Mode

```python
DRY_RUN = True

if DRY_RUN:
    print(f"[DRY RUN] Would delete image {digest}")
else:
    registry.delete_image(digest)
```

### 5. Progress Indicators

```python
images = registry.list_images()
total = len(images)

for i, image in enumerate(images):
    print(f"[{i+1}/{total}] Processing {image['name']}...")
    # Do work
```

---

## Advanced Usage

### Scheduled Execution

Use cron to run scripts on a schedule:

```cron
# Run cleanup daily at 2 AM
0 2 * * * ads-registry script /path/to/cleanup-old-images.star

# Generate weekly supply chain report
0 0 * * 0 ads-registry script /path/to/supply-chain-report.star
```

### Integration with CI/CD

**GitHub Actions:**

```yaml
- name: Generate K8s manifests
  run: |
    ads-registry script scripts/generate-k8s-manifests.star
    kubectl apply -f k8s-manifests/
```

**GitLab CI:**

```yaml
deploy:
  script:
    - ads-registry script scripts/migrate-compose-to-k8s.star
    - kubectl apply -f k8s-manifests/
```

### Combining with Other Tools

```python
# Use with kubectl
manifests = compose.to_k8s("docker-compose.yml")
with open("manifest.yaml", "w") as f:
    f.write(manifests)

# Then in shell:
# kubectl apply -f manifest.yaml
```

### Custom Modules

Create reusable modules:

**lib/helpers.star:**
```python
def is_production_image(image):
    """Check if image is tagged for production"""
    tags = image.get_tags(image["digest"])
    return "production" in tags or "prod" in tags

def get_high_severity_cves(scan_results):
    """Extract high and critical CVEs"""
    return [
        cve for cve in scan_results["vulnerabilities"]
        if cve["severity"] in ["HIGH", "CRITICAL"]
    ]
```

**main.star:**
```python
load("lib/helpers.star", "is_production_image", "get_high_severity_cves")

images = registry.list_images()
for image in images:
    if is_production_image(image):
        scan_results = scan.get_scan_results(image["digest"])
        high_cves = get_high_severity_cves(scan_results)
        if high_cves:
            print(f"⚠️  Production image {image['name']} has {len(high_cves)} high/critical CVEs!")
```

---

## Debugging

### Print Variables

```python
images = registry.list_images()
print(type(images))  # <class 'list'>
print(len(images))   # 42
print(images[0])     # First image details
```

### Dry Run Mode

Always test with dry run first:

```python
DRY_RUN = True

if DRY_RUN:
    print(f"Would delete: {image['name']}")
else:
    registry.delete_image(image["digest"])
```

### Verbose Output

```python
VERBOSE = True

if VERBOSE:
    print(f"Processing image: {image}")
    print(f"Tags: {tags}")
    print(f"Scan results: {scan_results}")
```

---

## Security Considerations

1. **Credential Management**: Never hardcode credentials
   ```python
   # ❌ Bad
   password = "mysecretpassword"

   # ✅ Good
   password = os.getenv("REGISTRY_PASSWORD")
   ```

2. **Validation**: Always validate user input
   ```python
   if not namespace.isalnum():
       fail("Invalid namespace name")
   ```

3. **Least Privilege**: Run scripts with minimal permissions needed

4. **Audit Logging**: Scripts are logged for auditing

---

## Performance Tips

1. **Batch Operations**: Use bulk APIs when available
2. **Caching**: Cache results that won't change
3. **Parallel Processing**: Use background tasks for independent operations
4. **Pagination**: Process large result sets in chunks

---

## Getting Help

- **Documentation**: `ads-registry script --help`
- **Examples**: `/usr/share/ads-registry/scripts/examples/`
- **Community**: https://github.com/ryan/ads-registry/discussions
- **Issues**: https://github.com/ryan/ads-registry/issues

---

## Reference

- **Starlark Language Spec**: https://github.com/bazelbuild/starlark
- **Starlark in Go**: https://github.com/google/starlark-go
- **Bazel Starlark Guide**: https://bazel.build/rules/language

---

**Happy Scripting!** 🚀
