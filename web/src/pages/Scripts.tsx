import { useState, useEffect } from 'react';
import { FileTerminal, Plus, Save, Trash2, Code, BookOpen, X } from 'lucide-react';
import CodeMirror from '@uiw/react-codemirror';
import { python } from '@codemirror/lang-python';
import { useAuth } from '../contexts/AuthContext';

export default function Scripts() {
  const { token } = useAuth();
  const [scripts, setScripts] = useState<string[]>([]);
  const [activeScript, setActiveScript] = useState<string | null>(null);
  const [code, setCode] = useState('');
  const [newScriptName, setNewScriptName] = useState('');
  const [showReference, setShowReference] = useState(false);

  const getHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  });

  const fetchScripts = () => {
    fetch('/api/v1/management/scripts', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => setScripts(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchScripts();
  }, []);

  const loadScript = (name: string) => {
    fetch(`/api/v1/management/scripts/${name}`, {
      headers: getHeaders()
    })
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
      headers: {
        'Content-Type': 'text/plain',
        'Authorization': `Bearer ${token}`
      },
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
      headers: {
        'Content-Type': 'text/plain',
        'Authorization': `Bearer ${token}`
      },
      body: '# New Starlark automation script\n\ndef on_push(event):\n    print("Image pushed:", event.data["repository"])'
    });
    setNewScriptName('');
    fetchScripts();
    loadScript(name);
  };

  const deleteScript = async (name: string) => {
    if (!confirm(`Delete ${name}?`)) return;
    await fetch(`/api/v1/management/scripts/${name}`, {
      method: 'DELETE',
      headers: getHeaders()
    });
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
        <div className="flex gap-2">
          <button
            onClick={() => setShowReference(true)}
            className="bg-secondary text-secondary-foreground hover:bg-secondary/80 px-4 py-2 rounded-md font-medium flex items-center shadow-sm text-sm border border-border"
          >
            <BookOpen className="w-4 h-4 mr-2" />
            Language Reference
          </button>
          <form onSubmit={createScript} className="flex gap-2">
            <input required value={newScriptName} onChange={e => setNewScriptName(e.target.value)} type="text" placeholder="e.g. post_push.star" className="bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" />
            <button type="submit" className="bg-primary text-primary-foreground hover:bg-primary/90 px-4 py-2 rounded-md font-medium flex items-center shadow-sm text-sm">
              <Plus className="w-4 h-4 mr-2" />
              New Script
            </button>
          </form>
        </div>
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

      {/* Language Reference Modal */}
      {showReference && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-card border border-border rounded-lg max-w-4xl w-full max-h-[90vh] overflow-hidden flex flex-col">
            <div className="px-6 py-4 border-b border-border flex items-center justify-between">
              <h2 className="text-xl font-bold text-foreground flex items-center gap-2">
                <BookOpen className="w-6 h-6" />
                Starlark Language Reference
              </h2>
              <button
                onClick={() => setShowReference(false)}
                className="p-2 hover:bg-muted rounded-lg transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="p-6 overflow-y-auto flex-1">
              <div className="space-y-6">
                {/* Introduction */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-2">What is Starlark?</h3>
                  <p className="text-sm text-muted-foreground">
                    Starlark is a Python-like scripting language designed for configuration and automation.
                    It's deterministic, hermetic, and safe for untrusted code execution.
                  </p>
                </section>

                {/* Basic Syntax */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-3">Basic Syntax</h3>
                  <div className="space-y-3">
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-1">Variables & Data Types</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`# Variables
name = "nginx"
tag = "latest"
count = 42
enabled = True

# Lists
tags = ["v1.0", "v1.1", "latest"]

# Dictionaries
metadata = {
    "author": "admin",
    "created": "2024-01-01",
    "labels": ["production", "public"]
}`}</pre>
                    </div>

                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-1">Control Flow</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`# If statements
if tag == "latest":
    print("Latest tag detected")
elif tag.startswith("v"):
    print("Version tag:", tag)
else:
    print("Unknown tag")

# For loops
for image in images:
    print(image["digest"])

# List comprehension
digests = [img["digest"] for img in images if img["size"] > 1000000]`}</pre>
                    </div>

                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-1">Functions</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`def notify_webhook(repo, tag):
    """Send webhook notification"""
    url = "https://hooks.example.com/registry"
    payload = {
        "repository": repo,
        "tag": tag,
        "timestamp": time.now()
    }
    http.post(url, json=payload)

def filter_large_images(images, max_size_mb):
    """Filter images larger than max_size_mb"""
    max_bytes = max_size_mb * 1024 * 1024
    return [img for img in images if img["size"] > max_bytes]`}</pre>
                    </div>
                  </div>
                </section>

                {/* Registry Events */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-3">Registry Event Hooks</h3>
                  <div className="space-y-3">
                    <div className="p-3 bg-blue-500/10 border border-blue-500/30 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-2">Available Event Hooks</p>
                      <div className="space-y-2 text-xs text-muted-foreground">
                        <div><code className="bg-background px-1.5 py-0.5 rounded">on_push(event)</code> - Called when an image is pushed</div>
                        <div><code className="bg-background px-1.5 py-0.5 rounded">on_pull(event)</code> - Called when an image is pulled</div>
                        <div><code className="bg-background px-1.5 py-0.5 rounded">on_delete(event)</code> - Called when an image is deleted</div>
                        <div><code className="bg-background px-1.5 py-0.5 rounded">on_scan_complete(event)</code> - Called when vulnerability scan completes</div>
                      </div>
                    </div>

                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-1">Event Object Structure</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`# event.data contains:
{
    "repository": "library/nginx",      # Full repository name
    "namespace": "library",              # Namespace
    "name": "nginx",                     # Image name
    "tag": "latest",                     # Tag (for push/pull)
    "digest": "sha256:abc123...",        # Content digest
    "media_type": "application/vnd...",  # Manifest media type
    "size": 12345678,                    # Size in bytes
    "user": "admin",                     # User who triggered event
    "timestamp": "2024-01-01T12:00:00Z"  # Event timestamp
}`}</pre>
                    </div>
                  </div>
                </section>

                {/* Built-in Functions */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-3">Registry Built-in Functions</h3>
                  <div className="space-y-2">
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">http.get(url, headers={})</code>
                      <p className="text-xs text-muted-foreground mt-1">Make HTTP GET request</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">http.post(url, json={}, headers={})</code>
                      <p className="text-xs text-muted-foreground mt-1">Make HTTP POST request with JSON body</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">registry.get_tags(repository)</code>
                      <p className="text-xs text-muted-foreground mt-1">Get all tags for a repository</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">registry.get_manifest(repository, reference)</code>
                      <p className="text-xs text-muted-foreground mt-1">Get manifest for a specific image</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">time.now()</code>
                      <p className="text-xs text-muted-foreground mt-1">Get current timestamp</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">json.encode(obj)</code>
                      <p className="text-xs text-muted-foreground mt-1">Encode object to JSON string</p>
                    </div>
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <code className="text-sm font-medium text-foreground">json.decode(str)</code>
                      <p className="text-xs text-muted-foreground mt-1">Decode JSON string to object</p>
                    </div>
                  </div>
                </section>

                {/* Example Scripts */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-3">Example Scripts</h3>
                  <div className="space-y-3">
                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-2">Webhook Notification on Push</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`def on_push(event):
    """Send webhook when production images are pushed"""
    repo = event.data["repository"]
    tag = event.data["tag"]

    # Only notify for production tags
    if tag in ["latest", "stable", "production"]:
        http.post("https://hooks.slack.com/...", json={
            "text": "New image pushed: {} ({})".format(repo, tag)
        })`}</pre>
                    </div>

                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-2">Auto-Tag Latest Images</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`def on_push(event):
    """Automatically tag version pushes as 'latest'"""
    repo = event.data["repository"]
    tag = event.data["tag"]

    # If pushing a version tag like v1.2.3
    if tag.startswith("v") and "." in tag:
        print("Version tag detected, updating latest")
        # Tag this image as 'latest' too
        registry.copy_tag(repo, tag, "latest")`}</pre>
                    </div>

                    <div className="p-3 bg-muted/50 rounded-lg">
                      <p className="text-sm font-medium text-foreground mb-2">Scan Large Images</p>
                      <pre className="text-xs text-muted-foreground font-mono bg-background p-2 rounded overflow-x-auto">
{`def on_push(event):
    """Trigger security scan for images over 100MB"""
    size_mb = event.data["size"] / (1024 * 1024)

    if size_mb > 100:
        repo = event.data["repository"]
        digest = event.data["digest"]
        print("Large image detected, triggering scan")
        registry.trigger_scan(repo, digest)`}</pre>
                    </div>
                  </div>
                </section>

                {/* Best Practices */}
                <section>
                  <h3 className="text-lg font-semibold text-foreground mb-3">Best Practices</h3>
                  <div className="space-y-2 text-sm text-muted-foreground">
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Keep scripts focused on a single task or event type</p>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Use descriptive function and variable names</p>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Add comments explaining complex logic</p>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Test scripts with small datasets before deploying</p>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Use error handling for external HTTP calls</p>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-1.5 h-1.5 rounded-full bg-primary mt-1.5 flex-shrink-0"></div>
                      <p>Avoid infinite loops or expensive operations</p>
                    </div>
                  </div>
                </section>
              </div>
            </div>

            <div className="px-6 py-4 border-t border-border flex justify-end">
              <button
                onClick={() => setShowReference(false)}
                className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
