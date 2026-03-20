import { useState, useEffect } from 'react';
import { Shield, AlertTriangle, AlertCircle, Info, ChevronDown, ChevronUp, ExternalLink } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

interface ScanReport {
  digest: string;
  scan_id: string;
  status: string;
  repository: string;
  tag: string;
  completed_at?: string;
  summary: {
    total_vulnerabilities: number;
    critical: number;
    high: number;
    medium: number;
    low: number;
    unknown: number;
    fixable: number;
  };
  vulnerabilities?: Vulnerability[];
  malware_found?: boolean;
  secrets_found?: Secret[];
}

interface Vulnerability {
  id: string;
  severity: string;
  package: string;
  version: string;
  fixed_in: string;
  title: string;
  description: string;
  links: string[];
}

interface Secret {
  type: string;
  file: string;
  line: number;
  description: string;
  severity: string;
}

export default function Vulnerabilities() {
  const { token } = useAuth();
  const [scans, setScans] = useState<ScanReport[]>([]);
  const [filter, setFilter] = useState('all');
  const [expandedScan, setExpandedScan] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const getHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  });

  const fetchData = () => {
    setLoading(true);
    fetch('/api/v1/management/scans', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => {
        setScans(data || []);
        setLoading(false);
      })
      .catch(err => {
        console.error('Failed to fetch scans:', err);
        setLoading(false);
      });
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000); // Refresh every 30s
    return () => clearInterval(interval);
  }, [token]);

  const filtered = scans.filter(scan => {
    if (filter === 'all') return true;
    if (filter === 'critical') return (scan.summary?.critical || 0) > 0;
    if (filter === 'high') return (scan.summary?.high || 0) > 0;
    if (filter === 'malware') return scan.malware_found === true;
    return true;
  });

  const getSeverityColor = (severity: string) => {
    switch (severity?.toUpperCase()) {
      case 'CRITICAL': return 'text-red-500 bg-red-500/10 border-red-500/30';
      case 'HIGH': return 'text-orange-500 bg-orange-500/10 border-orange-500/30';
      case 'MEDIUM': return 'text-yellow-600 bg-yellow-500/10 border-yellow-500/30';
      case 'LOW': return 'text-blue-500 bg-blue-500/10 border-blue-500/30';
      default: return 'text-muted-foreground bg-muted border-border';
    }
  };

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground flex items-center gap-2">
            <Shield className="w-7 h-7" />
            Vulnerability Scans
          </h1>
          <p className="text-muted-foreground mt-1">
            Security scan results from DarkScan via darkapi.io
          </p>
        </div>
      </div>

      <div className="flex gap-2 mb-6">
        <button
          onClick={() => setFilter('all')}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            filter === 'all' ? 'bg-primary text-primary-foreground' : 'bg-secondary hover:bg-secondary/80'
          }`}
        >
          All ({scans.length})
        </button>
        <button
          onClick={() => setFilter('critical')}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            filter === 'critical' ? 'bg-red-500 text-white' : 'bg-secondary hover:bg-secondary/80'
          }`}
        >
          Critical ({scans.filter(s => (s.summary?.critical || 0) > 0).length})
        </button>
        <button
          onClick={() => setFilter('high')}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            filter === 'high' ? 'bg-orange-500 text-white' : 'bg-secondary hover:bg-secondary/80'
          }`}
        >
          High ({scans.filter(s => (s.summary?.high || 0) > 0).length})
        </button>
        <button
          onClick={() => setFilter('malware')}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            filter === 'malware' ? 'bg-purple-500 text-white' : 'bg-secondary hover:bg-secondary/80'
          }`}
        >
          Malware ({scans.filter(s => s.malware_found).length})
        </button>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">
          Loading scans...
        </div>
      ) : filtered.length === 0 ? (
        <div className="bg-card border border-border rounded-lg p-12 text-center">
          <Shield className="w-16 h-16 mx-auto text-muted-foreground/50 mb-4" />
          <p className="text-muted-foreground">No vulnerability scans found.</p>
          <p className="text-sm text-muted-foreground mt-1">
            Scans are automatically performed when images are pushed to the registry.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {filtered.map((scan, idx) => {
            const isExpanded = expandedScan === scan.digest;

            return (
              <div key={idx} className="bg-card border border-border rounded-lg overflow-hidden">
                <div className="p-6">
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-3 mb-2">
                        <h3 className="font-semibold text-foreground">{scan.repository}:{scan.tag}</h3>
                        {scan.status === 'completed' ? (
                          <span className="px-2 py-0.5 text-xs bg-green-500/10 text-green-400 rounded border border-green-500/30">
                            Completed
                          </span>
                        ) : scan.status === 'scanning' ? (
                          <span className="px-2 py-0.5 text-xs bg-blue-500/10 text-blue-400 rounded border border-blue-500/30">
                            Scanning...
                          </span>
                        ) : (
                          <span className="px-2 py-0.5 text-xs bg-muted text-muted-foreground rounded">
                            {scan.status}
                          </span>
                        )}
                        {scan.malware_found && (
                          <span className="px-2 py-0.5 text-xs bg-purple-500/10 text-purple-400 rounded border border-purple-500/30 font-medium">
                            Malware Detected
                          </span>
                        )}
                      </div>
                      <div className="text-xs text-muted-foreground font-mono">
                        {scan.digest.substring(0, 71)}
                      </div>
                      {scan.completed_at && (
                        <div className="text-xs text-muted-foreground mt-1">
                          Scanned: {new Date(scan.completed_at).toLocaleString()}
                        </div>
                      )}
                    </div>
                    <button
                      onClick={() => setExpandedScan(isExpanded ? null : scan.digest)}
                      className="p-2 hover:bg-muted rounded-lg transition-colors"
                    >
                      {isExpanded ? (
                        <ChevronUp className="w-5 h-5" />
                      ) : (
                        <ChevronDown className="w-5 h-5" />
                      )}
                    </button>
                  </div>

                  <div className="grid grid-cols-5 gap-3">
                    <div className={`rounded-lg p-3 ${(scan.summary?.critical || 0) > 0 ? 'bg-red-500/10 border border-red-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertTriangle className={`w-4 h-4 ${(scan.summary?.critical || 0) > 0 ? 'text-red-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Critical</span>
                      </div>
                      <div className={`text-2xl font-bold ${(scan.summary?.critical || 0) > 0 ? 'text-red-500' : 'text-muted-foreground'}`}>
                        {scan.summary?.critical || 0}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${(scan.summary?.high || 0) > 0 ? 'bg-orange-500/10 border border-orange-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertCircle className={`w-4 h-4 ${(scan.summary?.high || 0) > 0 ? 'text-orange-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">High</span>
                      </div>
                      <div className={`text-2xl font-bold ${(scan.summary?.high || 0) > 0 ? 'text-orange-500' : 'text-muted-foreground'}`}>
                        {scan.summary?.high || 0}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${(scan.summary?.medium || 0) > 0 ? 'bg-yellow-500/10 border border-yellow-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertCircle className={`w-4 h-4 ${(scan.summary?.medium || 0) > 0 ? 'text-yellow-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Medium</span>
                      </div>
                      <div className={`text-2xl font-bold ${(scan.summary?.medium || 0) > 0 ? 'text-yellow-600' : 'text-muted-foreground'}`}>
                        {scan.summary?.medium || 0}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${(scan.summary?.low || 0) > 0 ? 'bg-blue-500/10 border border-blue-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <Info className={`w-4 h-4 ${(scan.summary?.low || 0) > 0 ? 'text-blue-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Low</span>
                      </div>
                      <div className={`text-2xl font-bold ${(scan.summary?.low || 0) > 0 ? 'text-blue-500' : 'text-muted-foreground'}`}>
                        {scan.summary?.low || 0}
                      </div>
                    </div>

                    <div className="rounded-lg p-3 bg-muted">
                      <div className="flex items-center gap-2 mb-1">
                        <Info className="w-4 h-4 text-muted-foreground" />
                        <span className="text-xs font-medium text-muted-foreground">Fixable</span>
                      </div>
                      <div className="text-2xl font-bold text-foreground">
                        {scan.summary?.fixable || 0}
                      </div>
                    </div>
                  </div>
                </div>

                {isExpanded && scan.vulnerabilities && scan.vulnerabilities.length > 0 && (
                  <div className="border-t border-border p-6 bg-muted/20">
                    <h4 className="font-semibold text-foreground mb-4">Vulnerability Details</h4>
                    <div className="space-y-3 max-h-96 overflow-y-auto">
                      {scan.vulnerabilities.slice(0, 20).map((vuln, vidx) => (
                        <div key={vidx} className="p-3 bg-card rounded-lg border border-border">
                          <div className="flex items-start justify-between gap-3">
                            <div className="flex-1">
                              <div className="flex items-center gap-2 mb-1">
                                <span className="font-mono text-sm font-medium text-foreground">{vuln.id}</span>
                                <span className={`px-2 py-0.5 text-xs rounded border font-medium ${getSeverityColor(vuln.severity)}`}>
                                  {vuln.severity}
                                </span>
                              </div>
                              <p className="text-sm text-foreground mb-1">{vuln.title || 'No title available'}</p>
                              <div className="text-xs text-muted-foreground space-y-1">
                                <div><strong>Package:</strong> {vuln.package} ({vuln.version})</div>
                                {vuln.fixed_in && <div><strong>Fixed in:</strong> {vuln.fixed_in}</div>}
                              </div>
                            </div>
                            {vuln.links && vuln.links.length > 0 && (
                              <a
                                href={vuln.links[0]}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="p-2 hover:bg-muted rounded-lg transition-colors"
                                title="View details"
                              >
                                <ExternalLink className="w-4 h-4 text-muted-foreground" />
                              </a>
                            )}
                          </div>
                        </div>
                      ))}
                      {scan.vulnerabilities.length > 20 && (
                        <div className="text-center text-sm text-muted-foreground py-2">
                          ... and {scan.vulnerabilities.length - 20} more vulnerabilities
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {isExpanded && scan.secrets_found && scan.secrets_found.length > 0 && (
                  <div className="border-t border-border p-6 bg-purple-500/5">
                    <h4 className="font-semibold text-foreground mb-4 flex items-center gap-2">
                      <AlertTriangle className="w-5 h-5 text-purple-500" />
                      Secrets Detected
                    </h4>
                    <div className="space-y-2">
                      {scan.secrets_found.map((secret, sidx) => (
                        <div key={sidx} className="p-3 bg-card rounded-lg border border-purple-500/30">
                          <div className="flex items-center gap-2 mb-1">
                            <span className={`px-2 py-0.5 text-xs rounded border font-medium ${getSeverityColor(secret.severity)}`}>
                              {secret.type}
                            </span>
                            <span className="text-xs text-muted-foreground font-mono">{secret.file}:{secret.line}</span>
                          </div>
                          <p className="text-sm text-foreground">{secret.description}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
