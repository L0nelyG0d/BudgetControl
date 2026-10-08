import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from '@/lib/auth'
import { PublicOnly, RequireAuth } from '@/components/AuthGate'
import { Layout } from '@/components/Layout'
import LoginPage from '@/pages/LoginPage'
import RegisterPage from '@/pages/RegisterPage'
import DashboardPage from '@/pages/DashboardPage'
import ExpensesPage from '@/pages/ExpensesPage'
import CategoriesPage from '@/pages/CategoriesPage'
import BudgetsPage from '@/pages/BudgetsPage'
import NotFoundPage from '@/pages/NotFoundPage'
import { MonthProvider } from '@/features/month/month'
import { Toaster } from '@/components/ui/sonner'

// The single routes definition: to build a page, edit its file in src/pages/.
function App() {
  return (
    <AuthProvider>
      <MonthProvider>
      <Routes>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route element={<PublicOnly />}>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
        </Route>
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/expenses" element={<ExpensesPage />} />
            <Route path="/categories" element={<CategoriesPage />} />
            <Route path="/budgets" element={<BudgetsPage />} />
          </Route>
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
      </MonthProvider>
      <Toaster />
    </AuthProvider>
  )
}

export default App
