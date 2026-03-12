# ADS Container Registry - Ansible Deployment

Automated deployment and management of the ADS Container Registry using Ansible.

## Directory Structure

```
ansible/
├── ansible.cfg                 # Ansible configuration
├── README.md                   # This file
├── inventory/
│   └── hosts.yml              # Inventory with server definitions
├── playbooks/
│   └── deploy-registry.yml    # Main deployment playbook
├── roles/
│   └── ads-registry/
│       ├── tasks/
│       │   └── main.yml       # Deployment tasks
│       ├── templates/
│       │   ├── config.json.j2            # Registry config template
│       │   └── ads-registry.service.j2   # Systemd service template
│       ├── files/             # Static files (if needed)
│       ├── handlers/          # Service handlers
│       └── vars/              # Role variables
└── scripts/
    ├── deploy.sh              # Full-featured deployment script
    └── quick-deploy.sh        # Quick deployment wrapper
```

## Prerequisites

1. **Ansible installed** on your local machine:
   ```bash
   pip install ansible
   ```

2. **SSH access** to the target server (configured in inventory)

3. **Go toolchain** (for building the binary locally)

## Quick Start

### Option 1: Using the deployment script (Recommended)

```bash
# Full deployment (build + deploy + restart)
cd ansible
./scripts/deploy.sh

# Deploy without building
./scripts/deploy.sh --no-build

# Dry-run (check mode)
./scripts/deploy.sh --check

# Verbose output
./scripts/deploy.sh -vv
```

### Option 2: Direct ansible-playbook

```bash
cd ansible
ansible-playbook -i inventory/hosts.yml playbooks/deploy-registry.yml
```

### Option 3: Quick deploy

```bash
cd ansible
./scripts/quick-deploy.sh
```

## Configuration

### Inventory Variables

Edit `inventory/hosts.yml` to configure your deployment:

```yaml
registry_servers:
  hosts:
    apps.afterdarksys.com:
      # Server connection
      ansible_host: apps.afterdarksys.com
      ansible_user: root

      # Installation paths
      registry_install_dir: /opt/ads-registry
      registry_data_dir: /opt/ads-registry/data

      # Network configuration
      registry_http_port: 5005
      registry_https_port: 5006

      # TLS configuration
      registry_tls_enabled: true
      registry_tls_cert: /path/to/cert.pem
      registry_tls_key: /path/to/key.pem

      # Database configuration
      registry_db_host: 127.0.0.1
      registry_db_port: 5434
      registry_db_name: ads_registry
      registry_db_user: ads_registry

      # Redis (optional)
      registry_redis_enabled: false
```

## Deployment Script Options

```
Usage: ./scripts/deploy.sh [OPTIONS]

OPTIONS:
    -h, --help              Show help message
    -i, --inventory FILE    Specify inventory file
    -p, --playbook FILE     Specify playbook file
    -n, --no-build          Don't build binary before deployment
    -r, --no-restart        Don't restart service after deployment
    -v, --verbose           Enable verbose Ansible output
    -vv, --very-verbose     Enable very verbose Ansible output
    --check                 Run in check mode (dry-run)
    --tags TAGS             Run only tasks with specific tags
    --skip-tags TAGS        Skip tasks with specific tags
```

## Common Tasks

### Deploy new version

```bash
cd ansible
./scripts/deploy.sh
```

### Deploy without restarting

```bash
./scripts/deploy.sh --no-restart
```

### Update only configuration

```bash
ansible-playbook -i inventory/hosts.yml playbooks/deploy-registry.yml \
  --tags config
```

### Restart service

```bash
ssh root@apps.afterdarksys.com 'systemctl restart ads-registry'
```

### Check service status

```bash
ssh root@apps.afterdarksys.com 'systemctl status ads-registry'
```

### View logs

```bash
ssh root@apps.afterdarksys.com 'journalctl -u ads-registry -f'
```

## Deployment Process

The deployment performs these steps:

1. **Pre-flight checks**: Verify Ansible installation and files
2. **Build binary**: Cross-compile for Linux AMD64 (optional)
3. **Create directories**: Ensure installation and data directories exist
4. **Copy binary**: Transfer compiled binary to server
5. **Configure**: Deploy config.json from template
6. **Install service**: Deploy systemd service file
7. **Start service**: Enable and start ads-registry service
8. **Health check**: Verify registry responds to /v2/ endpoint

## Troubleshooting

### Build fails

```bash
# Build manually
cd /Users/ryan/development/ads-registry
GOOS=linux GOARCH=amd64 go build -o ads-registry-linux ./cmd/ads-registry
```

### Service won't start

```bash
# Check logs
ssh root@apps.afterdarksys.com 'journalctl -u ads-registry -n 100 --no-pager'

# Check config
ssh root@apps.afterdarksys.com 'cat /opt/ads-registry/config.json | jq .'
```

### Connection issues

```bash
# Test SSH
ssh root@apps.afterdarksys.com 'echo OK'

# Test with Ansible
ansible -i inventory/hosts.yml registry_servers -m ping
```

### Ansible errors

```bash
# Run with verbose output
./scripts/deploy.sh -vv

# Check syntax
ansible-playbook --syntax-check playbooks/deploy-registry.yml
```

## Advanced Usage

### Deploy to multiple servers

Add more hosts to `inventory/hosts.yml`:

```yaml
registry_servers:
  hosts:
    apps1.afterdarksys.com:
      ...
    apps2.afterdarksys.com:
      ...
```

### Use Ansible Vault for secrets

```bash
# Create encrypted vars file
ansible-vault create inventory/vault.yml

# Add sensitive data
registry_db_password: "secret123"
registry_redis_password: "secret456"

# Deploy with vault
ansible-playbook -i inventory/hosts.yml playbooks/deploy-registry.yml \
  --ask-vault-pass
```

### Custom playbook

Create a custom playbook in `playbooks/`:

```yaml
---
- name: Custom Registry Deployment
  hosts: registry_servers
  vars:
    build_binary: yes
    custom_var: value

  roles:
    - ads-registry
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Deploy Registry
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Install Ansible
        run: pip install ansible
      - name: Deploy
        run: |
          cd ansible
          ./scripts/deploy.sh
        env:
          ANSIBLE_HOST_KEY_CHECKING: false
```

## Security Notes

- SSH keys should be properly secured
- Use Ansible Vault for sensitive variables
- Consider using a bastion host for production
- Rotate credentials regularly
- Review the systemd security settings in the service template

## Support

For issues or questions:
- Check the logs: `journalctl -u ads-registry`
- Verify configuration: `cat /opt/ads-registry/config.json`
- Test endpoint: `curl -I https://apps.afterdarksys.com/v2/`
