import { useState, useEffect } from 'react';
import { Shield, AlertTriangle, AlertCircle, Info, Search } from 'lucide-react';

export default function Vulnerabilities() {
  const [scans, setScans] = useState<any[]>([]);
  const [filter, setFilter] = useState('all');

  const fetchData = () => {
    fetch('/api/v1/management/scans')
      .then(res => res.json())
      .then(data => setScans(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchData();
  }, []);

  const getTotalVulns = (scan: any) => {
    return scan.critical + scan.high + scan.medium + scan.low;
  };

  const filtered = scans.filter(scan => {
    if (filter === 'all') return true;
    if (filter === 'critical') return scan.critical > 0;
    if (filter === 'high') return scan.high > 0;
    return true;
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Vulnerability Scans</h1>
          <p className="text-muted-foreground">Security scan results from Trivy scanner.</p>
        </div>
      </div>

      <div className="flex gap-2">
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
          Critical ({scans.filter(s => s.critical > 0).length})
        </button>
        <button
          onClick={() => setFilter('high')}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            filter === 'high' ? 'bg-orange-500 text-white' : 'bg-secondary hover:bg-secondary/80'
          }`}
        >
          High ({scans.filter(s => s.high > 0).length})
        </button>
      </div>

      <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-border bg-muted/20">
          <h3 className="font-semibold flex items-center">
            <Search className="w-5 h-5 mr-2 text-muted-foreground" />
            Scan Results ({filtered.length})
          </h3>
        </div>

        {filtered.length === 0 ? (
          <div className="p-12 text-center">
            <Shield className="w-12 h-12 mx-auto text-muted-foreground opacity-50 mb-3" />
            <p className="text-muted-foreground">No vulnerability scans found.</p>
            <p className="text-sm text-muted-foreground mt-1">
              Scans are automatically performed when images are pushed to the registry.
            </p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {filtered.map((scan: any, idx: number) => {
              const totalVulns = getTotalVulns(scan);
              return (
                <div key={idx} className="p-6 hover:bg-muted/5 transition-colors">
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex-1">
                      <div className="font-mono text-sm text-muted-foreground mb-2">
                        {scan.digest.substring(0, 19)}...
                      </div>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span className="px-2 py-0.5 bg-muted rounded font-medium">{scan.scanner}</span>
                        <span>{totalVulns} vulnerabilities found</span>
                      </div>
                    </div>
                  </div>

                  <div className="grid grid-cols-4 gap-3">
                    <div className={`rounded-lg p-3 ${scan.critical > 0 ? 'bg-red-500/10 border border-red-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertTriangle className={`w-4 h-4 ${scan.critical > 0 ? 'text-red-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Critical</span>
                      </div>
                      <div className={`text-2xl font-bold ${scan.critical > 0 ? 'text-red-500' : 'text-muted-foreground'}`}>
                        {scan.critical}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${scan.high > 0 ? 'bg-orange-500/10 border border-orange-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertCircle className={`w-4 h-4 ${scan.high > 0 ? 'text-orange-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">High</span>
                      </div>
                      <div className={`text-2xl font-bold ${scan.high > 0 ? 'text-orange-500' : 'text-muted-foreground'}`}>
                        {scan.high}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${scan.medium > 0 ? 'bg-yellow-500/10 border border-yellow-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <AlertCircle className={`w-4 h-4 ${scan.medium > 0 ? 'text-yellow-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Medium</span>
                      </div>
                      <div className={`text-2xl font-bold ${scan.medium > 0 ? 'text-yellow-600' : 'text-muted-foreground'}`}>
                        {scan.medium}
                      </div>
                    </div>

                    <div className={`rounded-lg p-3 ${scan.low > 0 ? 'bg-blue-500/10 border border-blue-500/30' : 'bg-muted'}`}>
                      <div className="flex items-center gap-2 mb-1">
                        <Info className={`w-4 h-4 ${scan.low > 0 ? 'text-blue-500' : 'text-muted-foreground'}`} />
                        <span className="text-xs font-medium text-muted-foreground">Low</span>
                      </div>
                      <div className={`text-2xl font-bold ${scan.low > 0 ? 'text-blue-500' : 'text-muted-foreground'}`}>
                        {scan.low}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
