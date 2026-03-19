import { useState, useEffect } from 'react';
import { Users as UsersIcon, Shield, Plus, UsersRound, Trash2, Edit, Key } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export default function Users() {
  const { token } = useAuth();
  const [users, setUsers] = useState<any[]>([]);
  const [groups, setGroups] = useState<any[]>([]);

  const [newUser, setNewUser] = useState({ username: '', password: '', scopes: '*' });
  const [newGroup, setNewGroup] = useState('');
  const [userGroupData, setUserGroupData] = useState({ username: '', groupName: '' });

  const [editingUser, setEditingUser] = useState<any>(null);
  const [resetPasswordUser, setResetPasswordUser] = useState<string>('');
  const [newPassword, setNewPassword] = useState('');

  const getHeaders = () => ({
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`
  });

  const fetchData = () => {
    fetch('/api/v1/management/users', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => setUsers(data || []))
      .catch(console.error);

    fetch('/api/v1/management/groups', {
      headers: getHeaders()
    })
      .then(res => res.json())
      .then(data => setGroups(data || []))
      .catch(console.error);
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newUser.username || !newUser.password) return;

    await fetch('/api/v1/management/users', {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({
        username: newUser.username,
        password: newUser.password,
        scopes: newUser.scopes.split(',').map(s => s.trim())
      })
    });
    setNewUser({ username: '', password: '', scopes: '*' });
    fetchData();
  };

  const handleCreateGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newGroup) return;

    await fetch('/api/v1/management/groups', {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({ name: newGroup })
    });
    setNewGroup('');
    fetchData();
  };

  const handleAddUserToGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!userGroupData.username || !userGroupData.groupName) return;

    await fetch(`/api/v1/management/groups/${encodeURIComponent(userGroupData.groupName)}/users`, {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({ username: userGroupData.username })
    });
    setUserGroupData({ username: '', groupName: '' });
    alert('User added to group successfully');
    fetchData();
  };

  const handleDeleteUser = async (username: string) => {
    if (!confirm(`Are you sure you want to delete user "${username}"?`)) return;

    await fetch(`/api/v1/management/users/${encodeURIComponent(username)}`, {
      method: 'DELETE',
      headers: getHeaders()
    });
    fetchData();
  };

  const handleUpdateUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingUser) return;

    await fetch(`/api/v1/management/users/${encodeURIComponent(editingUser.Username)}`, {
      method: 'PUT',
      headers: getHeaders(),
      body: JSON.stringify({
        scopes: editingUser.Scopes
      })
    });
    setEditingUser(null);
    fetchData();
  };

  const handleResetPassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!resetPasswordUser || !newPassword) return;

    await fetch(`/api/v1/management/users/${encodeURIComponent(resetPasswordUser)}/reset-password`, {
      method: 'POST',
      headers: getHeaders(),
      body: JSON.stringify({ password: newPassword })
    });
    setResetPasswordUser('');
    setNewPassword('');
    alert('Password reset successfully');
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Users & Groups</h1>
          <p className="text-muted-foreground">Manage roles, permissions, and group memberships.</p>
        </div>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-6">
          <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
            <div className="px-6 py-4 border-b border-border bg-muted/20">
              <h3 className="font-semibold flex items-center"><UsersIcon className="w-5 h-5 mr-2 text-muted-foreground"/> Existing Users</h3>
            </div>
            <div className="divide-y divide-border">
              {users.length === 0 ? (
                <div className="p-6 text-center text-muted-foreground">No users found.</div>
              ) : (
                users.map((user: any) => (
                  <div key={user.ID} className="p-4 px-6 hover:bg-muted/10 transition-colors">
                    {editingUser?.Username === user.Username ? (
                      <form onSubmit={handleUpdateUser} className="space-y-3">
                        <div>
                          <label className="block text-xs font-medium mb-1">Scopes (comma-separated)</label>
                          <input
                            value={editingUser.Scopes?.join(', ')}
                            onChange={e => setEditingUser({...editingUser, Scopes: e.target.value.split(',').map((s: string) => s.trim())})}
                            className="w-full bg-background border border-border rounded px-2 py-1 text-sm"
                          />
                        </div>
                        <div className="flex gap-2">
                          <button type="submit" className="bg-primary text-primary-foreground px-3 py-1 rounded text-sm">Save</button>
                          <button type="button" onClick={() => setEditingUser(null)} className="bg-secondary px-3 py-1 rounded text-sm">Cancel</button>
                        </div>
                      </form>
                    ) : (
                      <div className="flex items-center justify-between">
                        <div>
                          <div className="font-medium flex items-center">
                            {user.Username}
                            <Shield className="w-3 h-3 ml-2 text-primary opacity-70" />
                          </div>
                          <div className="text-xs text-muted-foreground mt-1 font-mono">Scopes: {user.Scopes?.join(', ')}</div>
                        </div>
                        <div className="flex gap-2">
                          <button
                            onClick={() => setEditingUser(user)}
                            className="p-1.5 hover:bg-muted rounded-md text-muted-foreground hover:text-foreground transition-colors"
                            title="Edit scopes"
                          >
                            <Edit className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => setResetPasswordUser(user.Username)}
                            className="p-1.5 hover:bg-muted rounded-md text-muted-foreground hover:text-foreground transition-colors"
                            title="Reset password"
                          >
                            <Key className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => handleDeleteUser(user.Username)}
                            className="p-1.5 hover:bg-destructive/10 rounded-md text-destructive hover:text-destructive transition-colors"
                            title="Delete user"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
            <div className="px-6 py-4 border-b border-border bg-muted/20">
              <h3 className="font-semibold flex items-center"><UsersIcon className="w-5 h-5 mr-2 text-muted-foreground"/> Create User</h3>
            </div>
            <form onSubmit={handleCreateUser} className="p-6 space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1 text-foreground">Username</label>
                <input required value={newUser.username} onChange={e => setNewUser(prev => ({...prev, username: e.target.value}))} type="text" className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent" placeholder="e.g. jdoe" />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1 text-foreground">Password</label>
                <input required value={newUser.password} onChange={e => setNewUser(prev => ({...prev, password: e.target.value}))} type="password" className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent" placeholder="••••••••" />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1 text-foreground">Scopes (comma-sep)</label>
                <input value={newUser.scopes} onChange={e => setNewUser(prev => ({...prev, scopes: e.target.value}))} type="text" className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent" placeholder="*" />
              </div>
              <button type="submit" className="w-full bg-primary text-primary-foreground hover:bg-primary/90 px-4 py-2 rounded-md font-medium flex justify-center items-center shadow-sm">
                <Plus className="w-4 h-4 mr-2" />
                Create User
              </button>
            </form>
          </div>
        </div>

        <div className="space-y-6">
          <div className="bg-card border border-border rounded-xl shadow-sm overflow-hidden">
            <div className="px-6 py-4 border-b border-border bg-muted/20">
              <h3 className="font-semibold flex items-center"><UsersRound className="w-5 h-5 mr-2 text-muted-foreground"/> Groups</h3>
            </div>
            
            <div className="divide-y divide-border">
              {groups.length === 0 ? (
                <div className="p-4 text-center text-muted-foreground text-sm">No groups found.</div>
              ) : (
                <div className="p-4 flex flex-wrap gap-2">
                  {groups.map((g: any) => (
                    <span key={g.ID} className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-secondary text-secondary-foreground border border-border">
                      {g.Name}
                    </span>
                  ))}
                </div>
              )}
            </div>

            <form onSubmit={handleCreateGroup} className="p-6 border-t border-border bg-muted/10">
              <div className="font-medium text-sm mb-3">Create Group</div>
              <div className="flex gap-2">
                <input required value={newGroup} onChange={e => setNewGroup(e.target.value)} type="text" className="flex-1 bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" placeholder="Group name" />
                <button type="submit" className="bg-primary text-primary-foreground hover:bg-primary/90 px-3 py-2 rounded-md font-medium text-sm">Add</button>
              </div>
            </form>

            <form onSubmit={handleAddUserToGroup} className="p-6 border-t border-border">
              <div className="font-medium text-sm mb-3">Add User to Group</div>
              <div className="space-y-3">
                <select required value={userGroupData.username} onChange={e => setUserGroupData(prev => ({...prev, username: e.target.value}))} className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary">
                  <option value="">Select User...</option>
                  {users.map(u => <option key={u.ID} value={u.Username}>{u.Username}</option>)}
                </select>
                <select required value={userGroupData.groupName} onChange={e => setUserGroupData(prev => ({...prev, groupName: e.target.value}))} className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary">
                  <option value="">Select Group...</option>
                  {groups.map(g => <option key={g.ID} value={g.Name}>{g.Name}</option>)}
                </select>
                <button type="submit" className="w-full bg-secondary text-secondary-foreground hover:bg-secondary/80 border border-border px-3 py-2 rounded-md font-medium text-sm">Assign User</button>
              </div>
            </form>
          </div>
        </div>
      </div>

      {resetPasswordUser && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setResetPasswordUser('')}>
          <div className="bg-card border border-border rounded-xl shadow-lg p-6 w-full max-w-md" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-semibold mb-4">Reset Password for {resetPasswordUser}</h3>
            <form onSubmit={handleResetPassword} className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">New Password</label>
                <input
                  type="password"
                  required
                  minLength={8}
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  className="w-full bg-background border border-border rounded-md px-3 py-2 text-sm"
                  placeholder="Min 8 characters"
                />
              </div>
              <div className="flex gap-2 justify-end">
                <button type="button" onClick={() => { setResetPasswordUser(''); setNewPassword(''); }} className="px-4 py-2 bg-secondary rounded-md text-sm">Cancel</button>
                <button type="submit" className="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm">Reset Password</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
