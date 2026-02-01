import { useQuery } from '@tanstack/react-query'
import axios from 'axios'

export function SystemMonitor() {
    const { data: stats, isLoading } = useQuery({
        queryKey: ['stats'],
        queryFn: async () => {
            const res = await axios.get('/api/stats')
            return res.data
        },
        refetchInterval: 5000 // Poll every 5 seconds
    })

    if (isLoading) return <div className="text-gray-400">Loading system stats...</div>

    return (
        <div>
            <h2 className="text-2xl font-bold mb-6">System Monitor</h2>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
                <StatCard title="CPU Usage" value={`${stats?.cpu}%`} color="text-green-400" />
                <StatCard title="Memory Usage" value={stats?.memory} color="text-blue-400" />
                <StatCard title="Disk Usage" value={stats?.disk} color="text-yellow-400" />
                <StatCard title="Uptime" value={stats?.uptime} color="text-purple-400" />
            </div>
        </div>
    )
}

function StatCard({ title, value, color }: { title: string, value: string, color: string }) {
    return (
        <div className="bg-gray-900 p-6 rounded-lg border border-gray-800">
            <div className="text-gray-400 text-sm mb-1">{title}</div>
            <div className={`text-3xl font-bold ${color}`}>{value}</div>
        </div>
    )
}
