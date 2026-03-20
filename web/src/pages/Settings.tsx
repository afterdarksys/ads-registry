import { useState, useEffect } from 'react';
import { Settings as SettingsIcon, Database, HardDrive, Shield, Zap, Globe, Bell, Info } from 'lucide-react';

interface SystemInfo {
  version: string;
  uptime: string;
  storage_driver: string;
  database_driver: string;
  redis_enabled: boolean;
  queue_enabled: boolean;
  oidc_enabled: boolean;
  vault_enabled: boolean;
}

export default function Settings() {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // For now, show placeholder system info
    // TODO: Add /api/v1/management/system/info endpoint
    setSystemInfo({
      version: 'v1.0.0',
      uptime: 'Loading...',
      storage_driver: 'oci',
      database_driver: 'postgres',
      redis_enabled: true,
      queue_enabled: true,
      oidc_enabled: true,
      vault_enabled: true,
    });
    setLoading(false);
  }, []);

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-foreground flex items-center gap-2">
          <SettingsIcon className="w-7 h-7" />
          Registry Settings
        </h1>
        <p className="text-muted-foreground mt-1">
          System configuration and status
        </p>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">
          Loading system information...
        </div>
      ) : (
        <div className="space-y-6">
          {/* System Information */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Info className="w-5 h-5" />
              System Information
            </h2>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <span className="text-sm text-muted-foreground">Version</span>
                <p className="text-foreground font-mono">{systemInfo?.version}</p>
              </div>
              <div>
                <span className="text-sm text-muted-foreground">Uptime</span>
                <p className="text-foreground">{systemInfo?.uptime}</p>
              </div>
            </div>
          </div>

          {/* Storage Configuration */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <HardDrive className="w-5 h-5" />
              Storage Backend
            </h2>
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium text-foreground">Driver</p>
                  <p className="text-sm text-muted-foreground">Current storage backend</p>
                </div>
                <span className="px-3 py-1 bg-primary/10 text-primary font-mono text-sm rounded">
                  {systemInfo?.storage_driver?.toUpperCase() || 'LOCAL'}
                </span>
              </div>
              {systemInfo?.storage_driver === 'oci' && (
                <div className="p-3 bg-blue-500/10 border border-blue-500/30 rounded-lg">
                  <p className="text-sm text-foreground">
                    <strong>Oracle Cloud Infrastructure</strong> Object Storage
                  </p>
                  <p className="text-xs text-muted-foreground mt-1">
                    Configured for production use with automatic encryption and lifecycle management
                  </p>
                </div>
              )}
            </div>
          </div>

          {/* Database Configuration */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Database className="w-5 h-5" />
              Database
            </h2>
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium text-foreground">Driver</p>
                <p className="text-sm text-muted-foreground">Database backend</p>
              </div>
              <span className="px-3 py-1 bg-primary/10 text-primary font-mono text-sm rounded">
                {systemInfo?.database_driver?.toUpperCase()}
              </span>
            </div>
          </div>

          {/* Features */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Zap className="w-5 h-5" />
              Enabled Features
            </h2>
            <div className="grid grid-cols-2 gap-4">
              {/* Redis Cache */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${systemInfo?.redis_enabled ? 'bg-green-500' : 'bg-gray-400'}`}></div>
                  <span className="text-sm font-medium text-foreground">Redis Cache</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {systemInfo?.redis_enabled ? 'Active' : 'Disabled'}
                </span>
              </div>

              {/* Job Queue */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${systemInfo?.queue_enabled ? 'bg-green-500' : 'bg-gray-400'}`}></div>
                  <span className="text-sm font-medium text-foreground">Job Queue</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {systemInfo?.queue_enabled ? 'Active' : 'Disabled'}
                </span>
              </div>

              {/* SSO/OIDC */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${systemInfo?.oidc_enabled ? 'bg-green-500' : 'bg-gray-400'}`}></div>
                  <span className="text-sm font-medium text-foreground">SSO (Authentik)</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {systemInfo?.oidc_enabled ? 'Active' : 'Disabled'}
                </span>
              </div>

              {/* Vault */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${systemInfo?.vault_enabled ? 'bg-green-500' : 'bg-gray-400'}`}></div>
                  <span className="text-sm font-medium text-foreground">HashiCorp Vault</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {systemInfo?.vault_enabled ? 'Active' : 'Disabled'}
                </span>
              </div>
            </div>
          </div>

          {/* Security */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Shield className="w-5 h-5" />
              Security & Authentication
            </h2>
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium text-foreground">Token Expiration</p>
                  <p className="text-sm text-muted-foreground">JWT token lifetime</p>
                </div>
                <span className="px-3 py-1 bg-muted text-foreground text-sm rounded">
                  72 hours
                </span>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium text-foreground">OIDC Provider</p>
                  <p className="text-sm text-muted-foreground">OAuth2 authentication</p>
                </div>
                <span className="px-3 py-1 bg-green-500/10 text-green-400 text-sm rounded">
                  Authentik SSO
                </span>
              </div>
            </div>
          </div>

          {/* Webhooks */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Bell className="w-5 h-5" />
              Webhooks & Integrations
            </h2>
            <div className="space-y-2">
              <p className="text-sm text-muted-foreground">
                Configured webhook endpoints for image push notifications
              </p>
              <div className="mt-3 p-3 bg-muted/50 rounded-lg">
                <p className="text-xs text-muted-foreground mb-1">Webhook Endpoints</p>
                <code className="text-xs text-foreground font-mono">
                  Coming soon: Manage webhooks via UI
                </code>
              </div>
            </div>
          </div>

          {/* Registry Endpoints */}
          <div className="bg-card border border-border rounded-lg p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4 flex items-center gap-2">
              <Globe className="w-5 h-5" />
              Registry Endpoints
            </h2>
            <div className="space-y-3">
              <div>
                <p className="text-sm font-medium text-foreground mb-1">Docker V2 API</p>
                <code className="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">
                  https://registry.afterdarksys.com/v2/
                </code>
              </div>
              <div>
                <p className="text-sm font-medium text-foreground mb-1">Management API</p>
                <code className="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">
                  https://registry.afterdarksys.com/api/v1/
                </code>
              </div>
              <div>
                <p className="text-sm font-medium text-foreground mb-1">OAuth2 Callback</p>
                <code className="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">
                  https://registry.afterdarksys.com/oauth2/callback
                </code>
              </div>
            </div>
          </div>

          {/* Info Banner */}
          <div className="p-4 bg-blue-500/10 border border-blue-500/30 rounded-lg">
            <p className="text-sm text-foreground">
              <strong>Note:</strong> Most settings are configured via <code className="px-1 py-0.5 bg-muted rounded">config.production.json</code> on the server.
              Advanced configuration options require server restart.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
