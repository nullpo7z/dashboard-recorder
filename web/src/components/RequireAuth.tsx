import { Navigate, useLocation } from 'react-router-dom'
import { ReactNode } from 'react'

export function RequireAuth({ children }: { children: ReactNode }) {
    const token = localStorage.getItem('token')
    const location = useLocation()

    if (!token) {
        return <Navigate to="/login" state={{ from: location }} replace />
    }

    return children
}
