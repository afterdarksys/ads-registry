
import { Outlet, NavLink } from 'react-router-dom';
import { Database, Shield, Settings, Server, Activity, User } from 'lucide-react';

const Layout = () => {
    return (
        <div style={{ display: 'flex', minHeight: '100vh' }}>
            {/* Sidebar */}
            <aside
                className="glass"
                style={{
                    width: '260px',
                    margin: '24px',
                    padding: '24px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '32px'
                }}
            >
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <div style={{ background: 'var(--accent-gradient)', padding: '8px', borderRadius: '12px' }}>
                        <Server size={24} color="white" />
                    </div>
                    <h2 className="text-gradient" style={{ fontSize: '1.25rem', margin: 0 }}>AdsRegistry</h2>
                </div>

                <nav style={{ display: 'flex', flexDirection: 'column', gap: '8px', flex: 1 }}>
                    <NavLink to="/" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
                        <Database size={20} /> Repositories
                    </NavLink>
                    <NavLink to="/policies" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
                        <Shield size={20} /> Policies
                    </NavLink>
                    <NavLink to="/activity" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
                        <Activity size={20} /> Activity
                    </NavLink>
                    <NavLink to="/settings" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
                        <Settings size={20} /> Settings
                    </NavLink>
                </nav>

                <div style={{
                    marginTop: 'auto',
                    paddingTop: '24px',
                    borderTop: '1px solid var(--border-glass)',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '12px'
                }}>
                    <div style={{ background: 'rgba(255,255,255,0.1)', padding: '8px', borderRadius: '50%' }}>
                        <User size={20} />
                    </div>
                    <div>
                        <div style={{ fontSize: '0.9rem', fontWeight: 500 }}>Admin User</div>
                        <div style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>admin@ads.local</div>
                    </div>
                </div>
            </aside>

            {/* Main Content */}
            <main style={{ flex: 1, padding: '24px 24px 24px 0', display: 'flex', flexDirection: 'column' }}>
                <div className="glass" style={{
                    padding: '16px 24px',
                    marginBottom: '24px',
                    display: 'flex',
                    justifyContent: 'flex-end',
                    alignItems: 'center'
                }}>
                    <div style={{ fontSize: '0.9rem', color: 'var(--text-muted)' }}>
                        Registry API v2.0 • Status: <span style={{ color: '#10b981' }}>Operational</span>
                    </div>
                </div>

                <div className="animate-fade-in" style={{ flex: 1 }}>
                    <Outlet />
                </div>
            </main>
        </div>
    );
};

export default Layout;
