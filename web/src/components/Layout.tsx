import { useState } from 'react'
import { Link, useLocation, Outlet } from 'react-router-dom'
import { Activity, LayoutDashboard, Video, FileVideo, LogOut, Lock } from 'lucide-react'
import { ChangePasswordModal } from './ChangePasswordModal'

export function Layout() {
    const [isPasswordModalOpen, setIsPasswordModalOpen] = useState(false)

    const handleLogout = () => {
        localStorage.removeItem('token')
        window.location.href = '/login'
    }

    return (
        <div className="flex h-screen bg-gray-950 text-white font-sans">
            <ChangePasswordModal
                isOpen={isPasswordModalOpen}
                onClose={() => setIsPasswordModalOpen(false)}
            />

            {/* Sidebar */}
            <aside className="w-64 border-r border-gray-800 p-4 flex flex-col gap-4">
                <h1 className="text-xl font-bold flex items-center gap-2">
                    <Video className="text-blue-500" />
                    Recorder
                </h1>
                <nav className="flex flex-col gap-1 flex-1">
                    <NavItem to="/" icon={<LayoutDashboard />} label="Dashboard" />
                    <NavItem to="/monitor" icon={<Activity />} label="Live Monitor" />
                    <NavItem to="/archives" icon={<FileVideo />} label="Archives" />
                </nav>

                <div className="border-t border-gray-800 pt-4 flex flex-col gap-2">
                    <button
                        onClick={() => setIsPasswordModalOpen(true)}
                        className="flex items-center gap-3 px-4 py-2 rounded-md hover:bg-gray-900 text-gray-400 transition-colors text-left"
                    >
                        <Lock className="w-5 h-5" />
                        <span>Change Password</span>
                    </button>
                    <button
                        onClick={handleLogout}
                        className="flex items-center gap-3 px-4 py-2 rounded-md hover:bg-red-900/20 text-red-400 transition-colors text-left"
                    >
                        <LogOut className="w-5 h-5" />
                        <span>Logout</span>
                    </button>
                </div>
            </aside>

            {/* Main Content */}
            <main className="flex-1 p-8 overflow-auto">
                <Outlet />
            </main>
        </div>
    )
}

function NavItem({ to, icon, label }: { to: string, icon: React.ReactNode, label: string }) {
    const location = useLocation()
    const active = location.pathname === to

    return (
        <Link to={to} className={`flex items-center gap-3 px-4 py-2 rounded-md transition-colors ${active ? 'bg-blue-600/20 text-blue-400' : 'hover:bg-gray-900 text-gray-400'}`}>
            {icon}
            <span>{label}</span>
        </Link>
    )
}
