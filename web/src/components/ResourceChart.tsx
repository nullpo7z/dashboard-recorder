import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { useResource } from '../context/ResourceContext'

export function ResourceChart() {
    const { history } = useResource()

    return (
        <div className="h-64 w-full" style={{ minHeight: '256px' }}>
            <ResponsiveContainer width="100%" height="100%" minWidth={0}>
                <LineChart data={history} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                    <XAxis
                        dataKey="time"
                        stroke="#9CA3AF"
                        tick={{ fill: '#9CA3AF', fontSize: 12 }}
                    />
                    <YAxis
                        stroke="#9CA3AF"
                        tick={{ fill: '#9CA3AF', fontSize: 12 }}
                        domain={[0, 100]}
                    />
                    <Tooltip
                        contentStyle={{
                            backgroundColor: '#1F2937',
                            border: '1px solid #374151',
                            borderRadius: '0.5rem'
                        }}
                        formatter={(value) => typeof value === 'number' ? `${value.toFixed(1)}%` : 'N/A'}
                    />
                    <Legend
                        wrapperStyle={{ color: '#9CA3AF' }}
                    />
                    <Line
                        type="monotone"
                        dataKey="cpu"
                        stroke="#3B82F6"
                        name="CPU"
                        strokeWidth={2}
                        dot={false}
                        isAnimationActive={false}
                    />
                    <Line
                        type="monotone"
                        dataKey="memory"
                        stroke="#10B981"
                        name="Memory"
                        strokeWidth={2}
                        dot={false}
                        isAnimationActive={false}
                    />
                    <Line
                        type="monotone"
                        dataKey="disk"
                        stroke="#F59E0B"
                        name="Disk"
                        strokeWidth={2}
                        dot={false}
                        isAnimationActive={false}
                    />
                </LineChart>
            </ResponsiveContainer>
        </div>
    )
}
