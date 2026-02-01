import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { Activity, Clock, HardDrive, Square } from 'lucide-react'
import { ResourceChart } from '../components/ResourceChart'
import { AuthenticatedImage } from '../components/AuthenticatedImage'
import { useState, useEffect } from 'react'

interface LiveRecording {
    id: number
    task_id: number
    task_name: string
    status: string
    elapsed_seconds: number
    file_size_bytes: number
    has_preview: boolean
}

export function LiveMonitor() {
    const queryClient = useQueryClient()
    const [previewTimestamp, setPreviewTimestamp] = useState(Date.now())

    // Request Interlock Polling:
    // Update timestamp every 500ms, but implicitly waits for re-renders.
    // This isn't a strict network interlock (which would require callback from child),
    // but it prevents stacking timers.
    useEffect(() => {
        const timer = setTimeout(() => {
            setPreviewTimestamp(Date.now())
        }, 500)
        return () => clearTimeout(timer)
    }, [previewTimestamp]) // Retrigger only after state update is processed

    const { data: recordings, isLoading } = useQuery({
        queryKey: ['live-recordings'],
        queryFn: async () => {
            const res = await axios.get('/api/recordings/live')
            return res.data as LiveRecording[]
        },
        refetchInterval: 2000,
    })

    const stopMutation = useMutation({
        mutationFn: async (taskID: number) => {
            await axios.post(`/api/tasks/${taskID}/stop`)
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['live-recordings'] })
        },
        onError: (err: any) => {
            alert("Failed to stop: " + (err.response?.data?.error || err.message))
        }
    })

    const formatDuration = (seconds: number) => {
        const mins = Math.floor(seconds / 60)
        const secs = seconds % 60
        return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
    }

    const formatBytes = (bytes: number) => {
        if (bytes === 0) return '0 B'
        const k = 1024
        const sizes = ['B', 'KB', 'MB', 'GB']
        const i = Math.floor(Math.log(bytes) / Math.log(k))
        return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
    }

    return (
        <div>
            <div className="flex justify-between items-center mb-6">
                <h2 className="text-2xl font-bold">Live Monitor</h2>
                <div className="text-sm text-gray-400">
                    {recordings?.length || 0} active recording(s)
                </div>
            </div>

            {/* Active Recordings Grid */}
            <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6 mb-8">
                {isLoading ? (
                    <div className="text-gray-400">Loading recordings...</div>
                ) : recordings && recordings.length > 0 ? (
                    recordings.map((rec: LiveRecording) => (
                        <div
                            key={rec.id}
                            className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden"
                        >
                            {/* Preview */}
                            <div className="relative aspect-video bg-gray-800 flex items-center justify-center">
                                {rec.has_preview ? (
                                    <AuthenticatedImage
                                        src={`/api/recordings/${rec.id}/preview.jpg?t=${previewTimestamp}`}
                                        alt={rec.task_name}
                                        className="w-full h-full object-contain"
                                    />
                                ) : (
                                    <div className="text-gray-600 text-sm">No preview available</div>
                                )}
                                <div className="absolute top-2 right-2 bg-red-500 text-white px-2 py-1 rounded text-xs font-medium flex items-center gap-1 animate-pulse">
                                    <div className="w-2 h-2 bg-white rounded-full animate-pulse"></div>
                                    RECORDING
                                </div>
                            </div>

                            {/* Info */}
                            <div className="p-4">
                                <div className="flex justify-between items-start mb-3">
                                    <h3 className="font-medium text-white truncate flex-1">{rec.task_name}</h3>
                                    <button
                                        onClick={() => stopMutation.mutate(rec.task_id)}
                                        disabled={stopMutation.isPending}
                                        className="ml-2 bg-amber-600/20 text-amber-500 hover:bg-amber-600/30 px-3 py-1 rounded flex items-center gap-1 transition-colors text-xs font-medium"
                                    >
                                        <Square className="w-3 h-3" /> Stop
                                    </button>
                                </div>

                                <div className="space-y-2">
                                    <div className="flex items-center gap-2 text-sm">
                                        <Clock className="w-4 h-4 text-blue-400" />
                                        <span className="text-gray-400">Duration:</span>
                                        <span className="text-white font-mono">{formatDuration(rec.elapsed_seconds)}</span>
                                    </div>

                                    <div className="flex items-center gap-2 text-sm">
                                        <HardDrive className="w-4 h-4 text-green-400" />
                                        <span className="text-gray-400">Size:</span>
                                        <span className="text-white font-mono">{formatBytes(rec.file_size_bytes)}</span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    ))
                ) : (
                    <div className="col-span-full text-center py-12">
                        <div className="text-gray-500">No active recordings</div>
                        <div className="text-gray-600 text-sm mt-2">Start a recording from the Dashboard</div>
                    </div>
                )}
            </div>

            {/* System Resources */}
            <div className="bg-gray-900 border border-gray-800 rounded-lg p-6">
                <div className="flex items-center gap-2 mb-4">
                    <Activity className="w-5 h-5 text-purple-400" />
                    <h3 className="text-lg font-medium">System Resources</h3>
                </div>
                <ResourceChart />
            </div>
        </div>
    )
}
