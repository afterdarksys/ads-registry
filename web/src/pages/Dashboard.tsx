
import { Box, HardDrive, ShieldAlert, Cpu } from 'lucide-react';

const Dashboard = () => {
    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end' }}>
                <div>
                    <h1 style={{ fontSize: '2rem', marginBottom: '8px' }}>Repositories</h1>
                    <p style={{ color: 'var(--text-muted)', margin: 0 }}>Manage container images and security policies.</p>
                </div>
                <button className="btn btn-primary">Create Repository</button>
            </div>

            {/* Metrics Row */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '24px' }}>
                {[
                    { icon: Box, label: 'Total Images', value: '142', color: '#60a5fa' },
                    { icon: HardDrive, label: 'Storage Used', value: '42.5 GB', color: '#a78bfa' },
                    { icon: ShieldAlert, label: 'Critical Vulns', value: '12', color: '#f87171' },
                    { icon: Cpu, label: 'Policy Blocks', value: '842', color: '#34d399' }
                ].map((metric, i) => (
                    <div key={i} className="glass-card" style={{ display: 'flex', alignItems: 'center', gap: '16px', padding: '20px' }}>
                        <div style={{ background: `rgba(255,255,255,0.05)`, padding: '12px', borderRadius: '12px', border: '1px solid var(--border-glass)' }}>
                            <metric.icon size={24} color={metric.color} />
                        </div>
                        <div>
                            <div style={{ fontSize: '1.5rem', fontWeight: 600, lineHeight: 1 }}>{metric.value}</div>
                            <div style={{ fontSize: '0.85rem', color: 'var(--text-muted)', marginTop: '4px' }}>{metric.label}</div>
                        </div>
                    </div>
                ))}
            </div>

            {/* Repositories List (Mock) */}
            <div className="glass-card" style={{ padding: 0, overflow: 'hidden' }}>
                <div style={{ padding: '20px 24px', borderBottom: '1px solid var(--border-glass)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <h3 style={{ margin: 0, fontSize: '1.1rem' }}>Active Repositories</h3>
                    <input type="text" className="glass-input" placeholder="Search..." style={{ width: '250px', padding: '8px 12px' }} />
                </div>

                <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
                    <thead>
                        <tr style={{ borderBottom: '1px solid var(--border-glass)' }}>
                            <th style={{ padding: '16px 24px', color: 'var(--text-muted)', fontWeight: 500, fontSize: '0.9rem' }}>Name</th>
                            <th style={{ padding: '16px 24px', color: 'var(--text-muted)', fontWeight: 500, fontSize: '0.9rem' }}>Tags</th>
                            <th style={{ padding: '16px 24px', color: 'var(--text-muted)', fontWeight: 500, fontSize: '0.9rem' }}>Size</th>
                            <th style={{ padding: '16px 24px', color: 'var(--text-muted)', fontWeight: 500, fontSize: '0.9rem' }}>Last Push</th>
                            <th style={{ padding: '16px 24px', color: 'var(--text-muted)', fontWeight: 500, fontSize: '0.9rem' }}>Security</th>
                        </tr>
                    </thead>
                    <tbody>
                        {[
                            { name: 'library/nginx', tags: 12, size: '1.2 GB', push: '2 hrs ago', sec: 'A+' },
                            { name: 'backend/api-service', tags: 45, size: '8.4 GB', push: '10 mins ago', sec: 'B' },
                            { name: 'frontend/webapp', tags: 8, size: '400 MB', push: '1 day ago', sec: 'A' },
                        ].map((row, i) => (
                            <tr key={i} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)', transition: 'background 0.2s' }}>
                                <td style={{ padding: '16px 24px', fontWeight: 500 }}>{row.name}</td>
                                <td style={{ padding: '16px 24px' }}>
                                    <span style={{ background: 'rgba(255,255,255,0.1)', padding: '4px 10px', borderRadius: '12px', fontSize: '0.85rem' }}>{row.tags} tags</span>
                                </td>
                                <td style={{ padding: '16px 24px', color: 'var(--text-secondary)' }}>{row.size}</td>
                                <td style={{ padding: '16px 24px', color: 'var(--text-secondary)' }}>{row.push}</td>
                                <td style={{ padding: '16px 24px' }}>
                                    <span style={{
                                        background: row.sec.includes('A') ? 'rgba(16, 185, 129, 0.1)' : 'rgba(245, 158, 11, 0.1)',
                                        color: row.sec.includes('A') ? '#34d399' : '#fbbf24',
                                        padding: '4px 10px', borderRadius: '12px', fontSize: '0.85rem', fontWeight: 600
                                    }}>Grade {row.sec}</span>
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

        </div>
    );
};

export default Dashboard;
