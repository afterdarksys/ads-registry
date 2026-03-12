import { useState, useEffect } from 'react';
import { Shield, Plus, Code } from 'lucide-react';
import CodeMirror from '@uiw/react-codemirror';
import { python } from '@codemirror/lang-python';

export default function Policies() {
  const [policies, setPolicies] = useState<any[]>([]);
  const [newExpression, setNewExpression] = useState('request.method == "POST" && request.namespace == "trusted"');

  const fetchPolicies = () => {
    fetch('/api/v1/management/policies')
      .then(res => res.json())
      .then(data => setPolicies(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchPolicies();
  }, []);

  const handleAddPolicy = async () => {
    if (!newExpression) return;
    await fetch('/api/v1/management/policies', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ expression: newExpression })
    });
    fetchPolicies();
    setNewExpression('');
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Security Policies</h1>
          <p className="text-muted-foreground">Manage CEL-based admission control policies for pushing and pulling images.</p>
        </div>
        <button onClick={handleAddPolicy} className="bg-primary text-primary-foreground hover:bg-primary/90 px-4 py-2 rounded-md font-medium flex items-center shadow-sm text-sm">
          <Plus className="w-4 h-4 mr-2" />
          Add Policy
        </button>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden flex flex-col h-full">
          <div className="px-6 py-4 border-b border-border bg-muted/20">
            <h3 className="font-semibold flex items-center"><Shield className="w-5 h-5 mr-2 text-muted-foreground"/> Active Policies</h3>
          </div>
          <div className="divide-y divide-border flex-1">
            {policies.length === 0 ? (
              <div className="p-6 text-center text-muted-foreground">No policies currently loaded.</div>
            ) : (
              policies.map((pol: any, idx: number) => (
                <div key={idx} className="p-4 px-6 hover:bg-muted/10 transition-colors">
                  <div className="font-mono text-sm break-words">{pol.Expression || JSON.stringify(pol)}</div>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden flex flex-col h-[400px]">
          <div className="px-6 py-4 border-b border-border bg-muted/20 flex justify-between items-center">
            <h3 className="font-semibold flex items-center"><Code className="w-5 h-5 mr-2 text-muted-foreground"/> New CEL Expression</h3>
          </div>
          <div className="flex-1 overflow-auto">
            <CodeMirror
              value={newExpression}
              height="100%"
              extensions={[python()]}
              onChange={(val) => setNewExpression(val)}
              theme="dark"
              className="h-full text-base"
            />
          </div>
        </div>
      </div>
    </div>
  );
}
