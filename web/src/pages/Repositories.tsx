import { useState, useEffect } from 'react';
import { Server, Database, Save } from 'lucide-react';

export default function Repositories() {
  const [repos, setRepos] = useState<string[]>([]);
  const [quotas, setQuotas] = useState<any[]>([]);
  const [newQuota, setNewQuota] = useState({ namespace: '', limitMb: 1024 });

  const fetchData = () => {
    fetch('/api/v1/management/repositories')
      .then(res => res.json())
      .then(data => setRepos(data || []))
      .catch(console.error);
      
    fetch('/api/v1/management/quotas')
      .then(res => res.json())
      .then(data => setQuotas(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleSetQuota = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newQuota.namespace || newQuota.limitMb < 0) return;
    
    await fetch('/api/v1/management/quotas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        namespace: newQuota.namespace,
        limit_bytes: newQuota.limitMb * 1024 * 1024
      })
    });
    setNewQuota({ namespace: '', limitMb: 1024 });
    fetchData();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Repositories & Quotas</h1>
          <p className="text-muted-foreground">Manage registry image repositories and storage limits.</p>
        </div>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-border bg-muted/20">
            <h3 className="font-semibold flex items-center"><Server className="w-5 h-5 mr-2 text-muted-foreground"/> Repositories</h3>
          </div>
          <div className="divide-y divide-border max-h-[500px] overflow-y-auto">
            {repos.length === 0 ? (
              <div className="p-6 text-center text-muted-foreground">No repositories found.</div>
            ) : (
              repos.map((repo, i) => (
                <div key={i} className="p-4 px-6 hover:bg-muted/10 transition-colors font-mono text-sm">
                  {repo}
                </div>
              ))
            )}
          </div>
        </div>
        
        <div className="space-y-6">
          <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden h-fit">
            <div className="px-6 py-4 border-b border-border bg-muted/20">
              <h3 className="font-semibold flex items-center"><Database className="w-5 h-5 mr-2 text-muted-foreground"/> Storage Quotas</h3>
            </div>
            <div className="divide-y divide-border">
              {quotas.length === 0 ? (
                <div className="p-6 text-center text-muted-foreground text-sm">No quotas defined.</div>
              ) : (
                quotas.map((q: any) => (
                  <div key={q.Namespace} className="p-4 px-6 hover:bg-muted/10 transition-colors">
                    <div className="font-medium mb-1 font-mono">{q.Namespace}</div>
                    <div className="w-full bg-muted rounded-full h-2.5 mb-1 overflow-hidden">
                      <div className="bg-primary h-2.5 rounded-full" style={{ width: `${Math.min(100, (q.UsedBytes / q.LimitBytes) * 100)}%` }}></div>
                    </div>
                    <div className="text-xs flex justify-between text-muted-foreground mt-2">
                      <span>{Math.round(q.UsedBytes / 1024 / 1024)} MB Used</span>
                      <span>{Math.round(q.LimitBytes / 1024 / 1024)} MB Limit</span>
                    </div>
                  </div>
                ))
              )}
            </div>
            
            <form onSubmit={handleSetQuota} className="p-6 border-t border-border bg-muted/10">
              <div className="font-medium text-sm mb-3">Set Namespace Quota</div>
              <div className="space-y-3">
                <input required value={newQuota.namespace} onChange={e => setNewQuota({...newQuota, namespace: e.target.value})} type="text" className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" placeholder="dev-team" />
                <div className="flex gap-2 items-center">
                  <input required value={newQuota.limitMb} onChange={e => setNewQuota({...newQuota, limitMb: parseInt(e.target.value) || 0})} type="number" className="flex-1 bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" placeholder="Limit in MB" />
                  <span className="text-sm font-medium text-muted-foreground w-10">MB</span>
                  <button type="submit" className="bg-primary text-primary-foreground hover:bg-primary/90 px-3 py-2 rounded-md font-medium text-sm flex items-center">
                    <Save className="w-4 h-4 mr-1"/>
                    Save
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}
