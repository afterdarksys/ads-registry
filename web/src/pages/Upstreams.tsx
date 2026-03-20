import { useState, useEffect } from 'react';
import { Cloud, RefreshCw, CheckCircle, XCircle, Clock } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export default function Upstreams() {
  const { token } = useAuth();
  const [upstreams, setUpstreams] = useState<any[]>([]);

  const getHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  });

  const fetchData = () => {
    fetch('/api/v1/management/upstreams', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => setUpstreams(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000); // Refresh every 30s
    return () => clearInterval(interval);
  }, []);

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'Never';
    const date = new Date(dateStr);
    return date.toLocaleString();
  };

  const getTypeColor = (type: string) => {
    switch (type?.toLowerCase()) {
      case 'aws_ecr': return 'bg-orange-500/10 text-orange-500 border-orange-500/30';
      case 'oracle_oci': return 'bg-red-500/10 text-red-500 border-red-500/30';
      case 'dockerhub': return 'bg-blue-500/10 text-blue-500 border-blue-500/30';
      case 'gcp': return 'bg-green-500/10 text-green-500 border-green-500/30';
      default: return 'bg-secondary text-secondary-foreground border-border';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Upstream Registries</h1>
          <p className="text-muted-foreground">Proxy and cache images from external registries.</p>
        </div>
        <button
          onClick={fetchData}
          className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90"
        >
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-border bg-muted/20">
          <h3 className="font-semibold flex items-center">
            <Cloud className="w-5 h-5 mr-2 text-muted-foreground" />
            Configured Upstreams ({upstreams.length})
          </h3>
        </div>

        {upstreams.length === 0 ? (
          <div className="p-12 text-center">
            <Cloud className="w-12 h-12 mx-auto text-muted-foreground opacity-50 mb-3" />
            <p className="text-muted-foreground">No upstream registries configured.</p>
            <p className="text-sm text-muted-foreground mt-1">
              Use the CLI to add upstreams: <code className="bg-muted px-2 py-0.5 rounded font-mono text-xs">ads-registry add-upstream</code>
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {upstreams.map((upstream: any) => (
              <div key={upstream.id} className="p-6 hover:bg-muted/5 transition-colors">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h4 className="font-semibold text-lg">{upstream.name}</h4>
                      <span className={`text-xs px-2 py-0.5 rounded-full border font-medium ${getTypeColor(upstream.type)}`}>
                        {upstream.type?.toUpperCase().replace('_', ' ')}
                      </span>
                      {upstream.enabled ? (
                        <CheckCircle className="w-4 h-4 text-green-500" />
                      ) : (
                        <XCircle className="w-4 h-4 text-red-500" />
                      )}
                    </div>

                    <div className="text-sm text-muted-foreground space-y-1">
                      <div className="font-mono">{upstream.endpoint}</div>
                      {upstream.region && (
                        <div className="flex items-center gap-2">
                          <span className="font-medium">Region:</span>
                          <span className="font-mono text-xs">{upstream.region}</span>
                        </div>
                      )}
                    </div>

                    <div className="mt-3 flex gap-4 text-xs text-muted-foreground">
                      <div className="flex items-center gap-1.5">
                        <Clock className="w-3.5 h-3.5" />
                        <span>Last refresh: {formatDate(upstream.last_refresh)}</span>
                      </div>
                      {upstream.cache_enabled && (
                        <div className="flex items-center gap-1.5">
                          <CheckCircle className="w-3.5 h-3.5 text-green-500" />
                          <span>Cache enabled</span>
                        </div>
                      )}
                      {upstream.pull_only && (
                        <div className="px-2 py-0.5 bg-muted rounded text-xs font-medium">
                          Pull Only
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="bg-blue-500/10 border border-blue-500/30 rounded-lg p-4 text-sm">
        <div className="font-medium text-blue-600 dark:text-blue-400 mb-1">CLI Management</div>
        <p className="text-blue-700 dark:text-blue-300">
          Upstream registries are managed via the CLI. Use the following commands:
        </p>
        <div className="mt-2 space-y-1 font-mono text-xs text-blue-600 dark:text-blue-400">
          <div>• <code className="bg-blue-500/10 px-1.5 py-0.5 rounded">ads-registry add-upstream --name my-ecr --type aws_ecr ...</code></div>
          <div>• <code className="bg-blue-500/10 px-1.5 py-0.5 rounded">ads-registry list-upstreams</code></div>
          <div>• <code className="bg-blue-500/10 px-1.5 py-0.5 rounded">ads-registry remove-upstream --name my-ecr</code></div>
        </div>
      </div>
    </div>
  );
}
