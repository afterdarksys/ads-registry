import { useState, useEffect } from 'react';
import { FileTerminal, Plus, Save, Trash2, Code } from 'lucide-react';
import CodeMirror from '@uiw/react-codemirror';
import { python } from '@codemirror/lang-python';

export default function Scripts() {
  const [scripts, setScripts] = useState<string[]>([]);
  const [activeScript, setActiveScript] = useState<string | null>(null);
  const [code, setCode] = useState('');
  const [newScriptName, setNewScriptName] = useState('');

  const fetchScripts = () => {
    fetch('/api/v1/management/scripts')
      .then(res => res.json())
      .then(data => setScripts(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchScripts();
  }, []);

  const loadScript = (name: string) => {
    fetch(`/api/v1/management/scripts/${name}`)
      .then(res => res.text())
      .then(text => {
        setCode(text);
        setActiveScript(name);
      })
      .catch(console.error);
  };

  const saveScript = async () => {
    if (!activeScript) return;
    await fetch(`/api/v1/management/scripts/${activeScript}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: code
    });
    alert('Saved successfully!');
  };

  const createScript = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newScriptName) return;
    let name = newScriptName;
    if (!name.endsWith('.star')) name += '.star';

    await fetch(`/api/v1/management/scripts/${name}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: '# New Starlark automation script\n\ndef on_push(event):\n    print("Image pushed:", event.data["repository"])'
    });
    setNewScriptName('');
    fetchScripts();
    loadScript(name);
  };

  const deleteScript = async (name: string) => {
    if (!confirm(`Delete ${name}?`)) return;
    await fetch(`/api/v1/management/scripts/${name}`, { method: 'DELETE' });
    if (activeScript === name) {
      setActiveScript(null);
      setCode('');
    }
    fetchScripts();
  };

  return (
    <div className="space-y-6 h-full flex flex-col">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Automation Scripts</h1>
          <p className="text-muted-foreground">Manage embedded Starlark (.star) routines triggered by registry events.</p>
        </div>
        <form onSubmit={createScript} className="flex gap-2">
          <input required value={newScriptName} onChange={e => setNewScriptName(e.target.value)} type="text" placeholder="e.g. post_push.star" className="bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" />
          <button type="submit" className="bg-primary text-primary-foreground hover:bg-primary/90 px-4 py-2 rounded-md font-medium flex items-center shadow-sm text-sm">
            <Plus className="w-4 h-4 mr-2" />
            New Script
          </button>
        </form>
      </div>

      <div className="grid gap-6 md:grid-cols-4 flex-1 min-h-[500px]">
        <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden flex flex-col">
          <div className="px-4 py-3 border-b border-border bg-muted/20">
            <h3 className="font-semibold text-sm flex items-center text-muted-foreground"><FileTerminal className="w-4 h-4 mr-2"/> Scripts</h3>
          </div>
          <div className="divide-y divide-border overflow-y-auto flex-1">
            {scripts.length === 0 ? (
              <div className="p-4 text-center text-muted-foreground text-sm">No scripts found.</div>
            ) : (
              scripts.map((script) => (
                <div key={script} onClick={() => loadScript(script)} className={`p-3 px-4 flex items-center justify-between cursor-pointer transition-colors ${activeScript === script ? 'bg-primary/10 border-l-2 border-primary' : 'hover:bg-muted/10'}`}>
                  <div className="font-mono text-sm truncate">{script}</div>
                  <button onClick={(e) => { e.stopPropagation(); deleteScript(script); }} className="text-destructive hover:bg-destructive/10 p-1.5 rounded-md transition-colors">
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="md:col-span-3 bg-card border border-border rounded-xl shadow-sm overflow-hidden flex flex-col">
          {activeScript ? (
            <>
              <div className="px-4 py-3 border-b border-border bg-muted/20 flex items-center justify-between">
                <div className="flex items-center text-sm font-medium">
                  <Code className="w-4 h-4 mr-2 text-primary" />
                  {activeScript}
                </div>
                <button onClick={saveScript} className="bg-secondary text-secondary-foreground hover:bg-secondary/80 px-3 py-1.5 rounded-md font-medium flex items-center text-xs shadow-sm border border-border">
                  <Save className="w-3.5 h-3.5 mr-1" />
                  Save Changes
                </button>
              </div>
              <div className="flex-1 overflow-auto bg-background/50">
                <CodeMirror
                  value={code}
                  height="100%"
                  extensions={[python()]}
                  onChange={(val) => setCode(val)}
                  theme="dark"
                  className="h-full text-base"
                />
              </div>
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground p-12 text-center">
              <FileTerminal className="w-12 h-12 mb-4 opacity-20" />
              <p>Select a script from the sidebar or create a new one to start editing.</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
