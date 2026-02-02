import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'

export function Login() {
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [error, setError] = useState<string | null>(null)
    const navigate = useNavigate()

    // Parse URL params for OIDC handling
    React.useEffect(() => {
        const params = new URLSearchParams(window.location.search)
        const token = params.get('token')
        const errorParam = params.get('error')

        if (token) {
            localStorage.setItem('token', token)
            navigate('/', { replace: true })
        }

        if (errorParam) {
            // Map common OIDC errors to user-friendly messages
            const errorMap: Record<string, string> = {
                'access_denied': 'Access Denied: You are not authorized to access this application.',
                'oidc_disabled': 'SSO is not currently enabled.',
                'invalid_state': 'Security Error: Invalid state parameter. Please try again.',
                'token_exchange_failed': 'Authentication failed during token exchange.',
                'invalid_token': 'Authentication failed: Invalid token.',
            }
            setError(errorMap[errorParam] || `Authentication Error: ${errorParam}`)
            // Clean URL
            window.history.replaceState({}, document.title, window.location.pathname)
        }
    }, [navigate])

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault()
        setError(null)
        try {
            const res = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password })
            })
            if (res.ok) {
                const data = await res.json()
                localStorage.setItem('token', data.token)
                navigate('/')
            } else {
                setError('Login failed: Invalid credentials')
            }
        } catch (err) {
            setError('Error logging in')
        }
    }

    return (
        <div className="min-h-screen flex items-center justify-center bg-gray-950 text-white p-4">
            <div className="w-full max-w-md space-y-4">
                {error && (
                    <div className="p-4 rounded-md bg-red-900/30 border border-red-800 text-red-200 text-sm font-medium">
                        {error}
                    </div>
                )}

                <form onSubmit={handleLogin} className="bg-gray-900 p-8 rounded-lg border border-gray-800 shadow-xl">
                    <h2 className="text-2xl font-bold mb-6 text-center text-gray-100">Dashboard Recorder</h2>
                    <div className="space-y-4">
                        <div>
                            <label className="block text-sm text-gray-400 mb-1">Username</label>
                            <input
                                type="text"
                                value={username}
                                onChange={e => setUsername(e.target.value)}
                                className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white placeholder-gray-600 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors"
                            />
                        </div>
                        <div>
                            <label className="block text-sm text-gray-400 mb-1">Password</label>
                            <input
                                type="password"
                                value={password}
                                onChange={e => setPassword(e.target.value)}
                                className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white placeholder-gray-600 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors"
                            />
                        </div>
                        <button type="submit" className="w-full bg-blue-600 hover:bg-blue-500 py-2 rounded font-medium transition-colors text-white shadow-lg shadow-blue-900/20">
                            Sign in with Password
                        </button>
                    </div>

                    <div className="mt-6">
                        <div className="relative">
                            <div className="absolute inset-0 flex items-center">
                                <div className="w-full border-t border-gray-800"></div>
                            </div>
                            <div className="relative flex justify-center text-sm">
                                <span className="px-2 bg-gray-900 text-gray-400">Or continue with</span>
                            </div>
                        </div>

                        <div className="mt-6">
                            <a
                                href="/auth/login"
                                className="w-full flex items-center justify-center py-2 px-4 border border-gray-700 rounded-md shadow-sm text-sm font-medium text-gray-200 bg-gray-800 hover:bg-gray-750 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-gray-900 focus:ring-gray-500 transition-colors"
                            >
                                <svg className="h-5 w-5 mr-2" fill="currentColor" viewBox="0 0 24 24">
                                    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z" />
                                </svg>
                                Sign in with SSO
                            </a>
                        </div>
                    </div>
                </form>
            </div>
        </div>
    )
}
