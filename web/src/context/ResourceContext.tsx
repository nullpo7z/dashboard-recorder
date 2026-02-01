import React, { createContext, useContext, useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import axios from 'axios'

interface SystemStats {
    cpu_percent: number
    memory_percent: number
    disk_percent: number
    timestamp: number
}

interface DataPoint {
    time: string
    cpu: number
    memory: number
    disk: number
}

interface ResourceContextType {
    history: DataPoint[]
}

const ResourceContext = createContext<ResourceContextType | undefined>(undefined)

export function ResourceProvider({ children }: { children: React.ReactNode }) {
    const [history, setHistory] = useState<DataPoint[]>([])

    const { data: stats } = useQuery({
        queryKey: ['system-stats'],
        queryFn: async () => {
            const res = await axios.get('/api/stats')
            return res.data as SystemStats
        },
        refetchInterval: 2000,
        // Keep polling in background so history builds up even when not looking at chart
    })

    useEffect(() => {
        if (stats) {
            const now = new Date()
            const timeStr = now.toLocaleTimeString('en-US', {
                hour12: false,
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            })

            setHistory((prev) => {
                const newHistory = [
                    ...prev,
                    {
                        time: timeStr,
                        cpu: stats.cpu_percent,
                        memory: stats.memory_percent,
                        disk: stats.disk_percent,
                    },
                ]
                return newHistory.slice(-30) // Keep last 1 minute
            })
        }
    }, [stats])

    return (
        <ResourceContext.Provider value={{ history }}>
            {children}
        </ResourceContext.Provider>
    )
}

export function useResource() {
    const context = useContext(ResourceContext)
    if (!context) {
        throw new Error('useResource must be used within a ResourceProvider')
    }
    return context
}
