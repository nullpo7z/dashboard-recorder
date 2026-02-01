import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Route, Routes, Navigate } from 'react-router-dom'
import { Dashboard } from './pages/Dashboard'
import { Login } from './pages/Login'
import { LiveMonitor } from './pages/LiveMonitor'
import { Archives } from './pages/Archives'
import { Layout } from './components/Layout'
import { Toaster } from 'sonner'
import { ResourceProvider } from './context/ResourceContext'
import { RequireAuth } from './components/RequireAuth'

const queryClient = new QueryClient()

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={
            <RequireAuth>
              <ResourceProvider>
                <Layout />
              </ResourceProvider>
            </RequireAuth>
          }>
            <Route path="/" element={<Dashboard />} />
            <Route path="/monitor" element={<LiveMonitor />} />
            <Route path="/archives" element={<Archives />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
      <Toaster />
    </QueryClientProvider>
  )
}

export default App
