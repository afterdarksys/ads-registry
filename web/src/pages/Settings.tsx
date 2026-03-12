import { Settings as SettingsIcon } from 'lucide-react';

export default function Settings() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Registry Settings</h1>
          <p className="text-muted-foreground">Global configuration for the ADS Container Registry.</p>
        </div>
      </div>

      <div className="bg-card border border-border rounded-xl shadow-sm p-12 text-center text-muted-foreground">
        <SettingsIcon className="w-12 h-12 mx-auto mb-4 opacity-20" />
        <p>Global settings form coming soon.</p>
      </div>
    </div>
  );
}
