import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="policies" element={<div className="glass-card"><h1>Policies</h1><p>CEL Admission policies and Starlark routines.</p></div>} />
          <Route path="activity" element={<div className="glass-card"><h1>Activity</h1><p>Registry audit logs.</p></div>} />
          <Route path="settings" element={<div className="glass-card"><h1>Settings</h1><p>Global registry configuration.</p></div>} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
