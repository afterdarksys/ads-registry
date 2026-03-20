import { useState, useEffect } from 'react';
import { Key, Plus, Trash2, Copy, CheckCircle2, Terminal } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

interface AccessToken {
  id: number;
  name: string;
  scopes: string[];
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
}

interface NewTokenResponse {
  id: number;
  name: string;
  token: string;
  expires_at?: string;
  docker_login: {
    registry: string;
    username: string;
    password: string;
    command: string;
  };
}

export default function AccessKeys() {
  const { token } = useAuth();
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newTokenName, setNewTokenName] = useState('');
  const [newTokenExpiry, setNewTokenExpiry] = useState('0');
  const [createdToken, setCreatedToken] = useState<NewTokenResponse | null>(null);
  const [copiedToken, setCopiedToken] = useState(false);
  const [copiedCommand, setCopiedCommand] = useState(false);

  const getHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  });

  const fetchTokens = () => {
    setLoading(true);
    fetch('/api/v1/management/access-tokens', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => {
        setTokens(data || []);
        setLoading(false);
      })
      .catch(err => {
        console.error('Failed to fetch access tokens:', err);
        setLoading(false);
      });
  };

  useEffect(() => {
    fetchTokens();
  }, [token]);

  const createToken = () => {
    if (!newTokenName.trim()) {
      alert('Please enter a token name');
      return;
    }

    const expiresIn = parseInt(newTokenExpiry);
    fetch('/api/v1/management/access-tokens', {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({
        name: newTokenName,
        expires_in: expiresIn > 0 ? expiresIn : 0
      })
    })
      .then(res => res.json())
      .then((data: NewTokenResponse) => {
        setCreatedToken(data);
        setShowCreateModal(false);
        setNewTokenName('');
        setNewTokenExpiry('0');
        fetchTokens();
      })
      .catch(err => {
        console.error('Failed to create token:', err);
        alert('Failed to create access token');
      });
  };

  const deleteToken = (id: number, name: string) => {
    if (!confirm(`Are you sure you want to delete token "${name}"? This cannot be undone.`)) {
      return;
    }

    fetch(`/api/v1/management/access-tokens/${id}`, {
      method: 'DELETE',
      headers: getHeaders()
    })
      .then(() => {
        fetchTokens();
      })
      .catch(err => {
        console.error('Failed to delete token:', err);
        alert('Failed to delete access token');
      });
  };

  const copyToClipboard = (text: string, isCommand: boolean = false) => {
    navigator.clipboard.writeText(text).then(() => {
      if (isCommand) {
        setCopiedCommand(true);
        setTimeout(() => setCopiedCommand(false), 2000);
      } else {
        setCopiedToken(true);
        setTimeout(() => setCopiedToken(false), 2000);
      }
    });
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return 'Never';
    return new Date(dateStr).toLocaleString();
  };

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground flex items-center gap-2">
            <Key className="w-7 h-7" />
            Access Keys
          </h1>
          <p className="text-muted-foreground mt-1">
            Manage access tokens for Docker CLI and automation
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90"
        >
          <Plus className="w-5 h-5" />
          Create Token
        </button>
      </div>

      {/* Info Banner */}
      <div className="mb-6 p-4 bg-blue-500/10 border border-blue-500/30 rounded-lg">
        <p className="text-sm text-foreground">
          <strong>Docker CLI Authentication:</strong> Use these access tokens to authenticate Docker CLI without OAuth2.
          Your Docker username is <code className="px-1 py-0.5 bg-muted rounded">username-oci</code> and the password is your access token.
        </p>
      </div>

      {/* Token List */}
      {loading ? (
        <div className="text-center py-12 text-muted-foreground">
          Loading tokens...
        </div>
      ) : tokens.length === 0 ? (
        <div className="text-center py-12">
          <Key className="w-16 h-16 mx-auto mb-4 text-muted-foreground/50" />
          <p className="text-muted-foreground">No access tokens yet</p>
          <p className="text-sm text-muted-foreground mt-1">
            Create an access token to authenticate Docker CLI
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {tokens.map(token => (
            <div key={token.id} className="p-4 bg-card border border-border rounded-lg">
              <div className="flex items-center justify-between">
                <div className="flex-1">
                  <div className="flex items-center gap-3">
                    <h3 className="font-semibold text-foreground">{token.name}</h3>
                    {token.expires_at && new Date(token.expires_at) < new Date() && (
                      <span className="px-2 py-0.5 text-xs bg-red-500/20 text-red-400 rounded">
                        Expired
                      </span>
                    )}
                  </div>
                  <div className="mt-2 grid grid-cols-3 gap-4 text-sm">
                    <div>
                      <span className="text-muted-foreground">Created:</span>
                      <span className="ml-2 text-foreground">{formatDate(token.created_at)}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground">Last Used:</span>
                      <span className="ml-2 text-foreground">{formatDate(token.last_used_at)}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground">Expires:</span>
                      <span className="ml-2 text-foreground">
                        {token.expires_at ? formatDate(token.expires_at) : 'Never'}
                      </span>
                    </div>
                  </div>
                </div>
                <button
                  onClick={() => deleteToken(token.id, token.name)}
                  className="p-2 text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
                  title="Delete token"
                >
                  <Trash2 className="w-5 h-5" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create Token Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card border border-border rounded-lg p-6 max-w-md w-full mx-4">
            <h2 className="text-xl font-bold text-foreground mb-4">Create Access Token</h2>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-foreground mb-1">
                  Token Name
                </label>
                <input
                  type="text"
                  value={newTokenName}
                  onChange={(e) => setNewTokenName(e.target.value)}
                  placeholder="e.g., ci-pipeline, laptop, prod-server"
                  className="w-full px-3 py-2 bg-background border border-border rounded-lg text-foreground"
                  autoFocus
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-foreground mb-1">
                  Expiration
                </label>
                <select
                  value={newTokenExpiry}
                  onChange={(e) => setNewTokenExpiry(e.target.value)}
                  className="w-full px-3 py-2 bg-background border border-border rounded-lg text-foreground"
                >
                  <option value="0">Never</option>
                  <option value="7">7 days</option>
                  <option value="30">30 days</option>
                  <option value="90">90 days</option>
                  <option value="365">1 year</option>
                </select>
              </div>
            </div>

            <div className="flex gap-3 mt-6">
              <button
                onClick={createToken}
                className="flex-1 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90"
              >
                Create Token
              </button>
              <button
                onClick={() => {
                  setShowCreateModal(false);
                  setNewTokenName('');
                  setNewTokenExpiry('0');
                }}
                className="px-4 py-2 bg-muted text-foreground rounded-lg hover:bg-muted/80"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Created Token Display Modal */}
      {createdToken && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-card border border-border rounded-lg p-6 max-w-2xl w-full mx-4">
            <div className="flex items-center gap-2 mb-4">
              <CheckCircle2 className="w-6 h-6 text-green-500" />
              <h2 className="text-xl font-bold text-foreground">Access Token Created</h2>
            </div>

            <div className="mb-4 p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
              <p className="text-sm text-yellow-200">
                <strong>Important:</strong> Copy this token now. You won't be able to see it again!
              </p>
            </div>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-foreground mb-1">
                  Access Token
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={createdToken.token}
                    readOnly
                    className="flex-1 px-3 py-2 bg-background border border-border rounded-lg text-foreground font-mono text-sm"
                  />
                  <button
                    onClick={() => copyToClipboard(createdToken.token)}
                    className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 flex items-center gap-2"
                  >
                    {copiedToken ? (
                      <>
                        <CheckCircle2 className="w-4 h-4" />
                        Copied!
                      </>
                    ) : (
                      <>
                        <Copy className="w-4 h-4" />
                        Copy
                      </>
                    )}
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-foreground mb-2 flex items-center gap-2">
                  <Terminal className="w-4 h-4" />
                  Docker Login Command
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={createdToken.docker_login.command}
                    readOnly
                    className="flex-1 px-3 py-2 bg-background border border-border rounded-lg text-foreground font-mono text-xs"
                  />
                  <button
                    onClick={() => copyToClipboard(createdToken.docker_login.command, true)}
                    className="px-4 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 flex items-center gap-2"
                  >
                    {copiedCommand ? (
                      <>
                        <CheckCircle2 className="w-4 h-4" />
                        Copied!
                      </>
                    ) : (
                      <>
                        <Copy className="w-4 h-4" />
                        Copy
                      </>
                    )}
                  </button>
                </div>
              </div>

              <div className="p-3 bg-muted/50 border border-border rounded-lg">
                <h4 className="font-medium text-foreground mb-2">Usage Instructions:</h4>
                <ol className="text-sm text-muted-foreground space-y-1 list-decimal list-inside">
                  <li>Copy the access token above</li>
                  <li>Run the Docker login command (or copy it to your CI/CD secrets)</li>
                  <li>Docker username: <code className="px-1 py-0.5 bg-background rounded">{createdToken.docker_login.username}</code></li>
                  <li>Docker password: Your access token</li>
                </ol>
              </div>
            </div>

            <button
              onClick={() => setCreatedToken(null)}
              className="w-full mt-6 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90"
            >
              Done
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
