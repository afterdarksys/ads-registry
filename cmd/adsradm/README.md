# adsradm - ADS Registry Remote Administration CLI

`adsradm` is a powerful command-line tool for remotely managing ADS Registry instances via the management API.

## Features

- **User Management**: Create, list, update, delete users and reset passwords
- **Upstream Registries**: View and manage upstream registry configurations (AWS ECR, Oracle OCI, Docker Hub)
- **Repository Management**: List repositories, tags, and manifests
- **Vulnerability Scanning**: View scan reports and vulnerability summaries
- **Policy Management**: List and add security policies
- **Quota Management**: Manage namespace storage quotas
- **Group Management**: Create groups and manage membership
- **Script Management**: Upload, view, and delete Starlark automation scripts
- **Statistics**: View registry statistics and metrics

## Installation

Build from source:

```bash
go build -o adsradm ./cmd/adsradm
```

Or use the pre-built binary from releases.

## Configuration

Create a configuration file at `~/.adsradm.yaml`:

```yaml
# Registry API URL
url: https://registry.example.com

# Admin authentication token
token: your-admin-token-here
```

Alternatively, use command-line flags:

```bash
adsradm --url https://registry.example.com --token <admin-token> <command>
```

### Getting an Admin Token

First, create an admin user on the registry server:

```bash
# On the registry server
ads-registry create-user admin --scopes=admin
```

Then, authenticate to get a token:

```bash
curl -X POST https://registry.example.com/v2/token \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

Save the returned token in your `~/.adsradm.yaml` configuration file.

## Usage Examples

### User Management

```bash
# List all users
adsradm users list

# Create a new user
adsradm users create myuser --scopes="repo:*,pull:*"

# Update user scopes
adsradm users update myuser --scopes="admin"

# Reset user password
adsradm users reset-password myuser

# Delete a user
adsradm users delete myuser
```

### Repository Management

```bash
# List all repositories
adsradm repos list

# List tags for a repository
adsradm repos tags myrepo/myimage

# List manifests for a repository
adsradm repos manifests myrepo/myimage
```

### Upstream Registries

```bash
# List all upstream registries
adsradm upstreams list
```

Note: To add upstream registries, use the main `ads-registry` CLI on the server:

```bash
ads-registry add-upstream my-ecr --type aws --aws-region us-west-2 \
  --aws-account-id 123456789012 --aws-access-key AKIA... --aws-secret-key ...
```

### Vulnerability Scanning

```bash
# List all scan reports
adsradm scans list

# Get detailed scan report for a digest
adsradm scans get sha256:abc123...

# Get scan report from specific scanner
adsradm scans get sha256:abc123... --scanner trivy
```

### Policy Management

```bash
# List all policies
adsradm policies list

# Add a new policy
adsradm policies add "critical_vulns < 5"
```

### Quota Management

```bash
# List all quotas
adsradm quotas list

# Set quota for a namespace (bytes)
adsradm quotas set mynamespace 10737418240  # 10 GiB
```

### Group Management

```bash
# List all groups
adsradm groups list

# Create a new group
adsradm groups create developers

# Add user to group
adsradm groups add-user developers alice
```

### Script Management

```bash
# List all scripts
adsradm scripts list

# View script content
adsradm scripts get my-script.star

# Upload a script
adsradm scripts upload my-script.star /path/to/local/script.star

# Delete a script
adsradm scripts delete my-script.star
```

### Statistics

```bash
# View registry statistics
adsradm stats
```

## Environment Variables

You can also use environment variables for configuration:

```bash
export ADSRADM_URL=https://registry.example.com
export ADSRADM_TOKEN=your-admin-token

adsradm users list
```

## Security

- Always use HTTPS for production deployments
- Store your admin token securely (use `chmod 600 ~/.adsradm.yaml`)
- Rotate admin tokens regularly
- Use minimal scopes for admin users when possible

## Troubleshooting

### "Registry URL not set"

Make sure you've either:
1. Created a `~/.adsradm.yaml` config file with the `url` field
2. Used the `--url` flag
3. Set the `ADSRADM_URL` environment variable

### "Admin token not set"

Make sure you've either:
1. Added the `token` field to your `~/.adsradm.yaml` config file
2. Used the `--token` flag
3. Set the `ADSRADM_TOKEN` environment variable

### API Errors

Check that:
1. The registry server is running and accessible
2. Your token is valid and has admin privileges
3. The management API is enabled on the server

## License

Same license as ADS Registry.
