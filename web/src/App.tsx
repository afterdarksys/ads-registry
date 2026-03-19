import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Users from './pages/Users';
import Repositories from './pages/Repositories';
import Policies from './pages/Policies';
import Scripts from './pages/Scripts';
import Settings from './pages/Settings';
import Upstreams from './pages/Upstreams';
import Vulnerabilities from './pages/Vulnerabilities';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="users" element={<Users />} />
          <Route path="repositories" element={<Repositories />} />
          <Route path="upstreams" element={<Upstreams />} />
          <Route path="vulnerabilities" element={<Vulnerabilities />} />
          <Route path="policies" element={<Policies />} />
          <Route path="scripts" element={<Scripts />} />
          <Route path="settings" element={<Settings />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
