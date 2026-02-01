import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'

export function Login() {
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const navigate = useNavigate()

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault()
        // Demo login implementation
        try {
            const res = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password })
            })
            if (res.ok) {
                const data = await res.json()
                localStorage.setItem('token', data.token) // In prod use cookie or secure storage context
                navigate('/')
            } else {
                alert('Login failed')
            }
        } catch (err) {
            alert('Error logging in')
        }
    }

    return (
        <div className="min-h-screen flex items-center justify-center bg-gray-950 text-white">
            <form onSubmit={handleLogin} className="w-full max-w-md bg-gray-900 p-8 rounded-lg border border-gray-800">
                <h2 className="text-2xl font-bold mb-6 text-center">Dashboard Recorder</h2>
                <div className="space-y-4">
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Username</label>
                        <input
                            type="text"
                            value={username}
                            onChange={e => setUsername(e.target.value)}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 focus:border-blue-500 focus:outline-none"
                        />
                    </div>
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Password</label>
                        <input
                            type="password"
                            value={password}
                            onChange={e => setPassword(e.target.value)}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 focus:border-blue-500 focus:outline-none"
                        />
                    </div>
                    <button type="submit" className="w-full bg-blue-600 hover:bg-blue-500 py-2 rounded font-medium transition-colors">
                        Login
                    </button>
                </div>
            </form>
        </div>
    )
}
