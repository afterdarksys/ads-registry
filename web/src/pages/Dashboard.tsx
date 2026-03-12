export default function Dashboard() {
  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold tracking-tight">Dashboard overview</h1>
      <p className="text-muted-foreground">General statistics and system health.</p>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {/* Placeholder cards */}
        {['Total Repositories', 'Storage Used', 'Critical Vulnerabilities', 'Policy Blocks'].map((title) => (
          <div key={title} className="bg-card border border-border rounded-xl p-6 shadow-sm">
            <h3 className="text-sm font-medium text-muted-foreground">{title}</h3>
            <div className="text-2xl font-bold mt-2">--</div>
          </div>
        ))}
      </div>
    </div>
  );
}
