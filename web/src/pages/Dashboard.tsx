import React from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Play, Square, MousePointer, Settings } from 'lucide-react'
import axios from 'axios'
import { InteractModal } from '../components/InteractModal'

interface Task {
    id: number
    name: string
    target_url: string
    is_enabled: boolean
    created_at: string
    filename_template: string
    custom_css: string
    fps: number
    crf: number
    time_overlay: boolean
    time_overlay_config: string
}

export function Dashboard() {
    const [isCreateModalOpen, setIsCreateModalOpen] = React.useState(false)
    const [editingTask, setEditingTask] = React.useState<Task | null>(null)
    const [interactingTask, setInteractingTask] = React.useState<Task | null>(null) // Interacting state
    const queryClient = useQueryClient()

    const createMutation = useMutation({
        mutationFn: async (newTask: { [key: string]: any }) => {
            const res = await axios.post('/api/tasks', newTask)
            return res.data
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['tasks'] })
            setIsCreateModalOpen(false)
        }
    })

    const updateMutation = useMutation({
        mutationFn: async (updatedTask: any) => {
            await axios.put(`/api/tasks/${updatedTask.id}`, updatedTask)
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['tasks'] })
            setEditingTask(null)
        }
    })

    const startMutation = useMutation({
        mutationFn: async (id: number) => {
            await axios.post(`/api/tasks/${id}/start`)
        },
        onSuccess: () => {
            // Optional: invalidate if we want immediate UI update beyond optimistic
            queryClient.invalidateQueries({ queryKey: ['tasks'] })
        },
        onError: (err: any) => {
            alert("Failed to start: " + (err.response?.data?.error || err.message))
        }
    })

    const stopMutation = useMutation({
        mutationFn: async (id: number) => {
            await axios.post(`/api/tasks/${id}/stop`)
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['tasks'] })
        },
        onError: (err: any) => {
            alert("Failed to stop: " + (err.response?.data?.error || err.message))
        }
    })

    const { data: tasks, isLoading, error } = useQuery({
        queryKey: ['tasks'],
        queryFn: async () => {
            const res = await axios.get('/api/tasks')
            return res.data
        }
    })

    if (error) return <div className="p-8 text-red-500">Error loading tasks: {(error as Error).message}</div>

    return (
        <div>
            <div className="flex justify-between items-center mb-6">
                <h2 className="text-2xl font-bold">Overview</h2>
                <button
                    onClick={() => setIsCreateModalOpen(true)}
                    className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg flex items-center gap-2 transition-colors"
                >
                    <Plus className="w-4 h-4" />
                    New Recording Task
                </button>
            </div>

            {/* Tasks Table */}
            <div className="bg-gray-900 rounded-lg p-6 border border-gray-800">
                <div className="flex justify-between items-center mb-4">
                    <h3 className="text-lg font-semibold">Recording Tasks</h3>
                </div>

                {isLoading ? (
                    <div className="text-gray-400 text-sm">Loading tasks...</div>
                ) : tasks && tasks.length > 0 ? (
                    <div className="overflow-x-auto">
                        <table className="w-full text-left text-sm opacity-90">
                            <thead className="text-gray-400 border-b border-gray-800">
                                <tr>
                                    <th className="pb-3">Name</th>
                                    <th className="pb-3">Target URL</th>
                                    <th className="pb-3 text-right">Actions</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-800">
                                {tasks?.map((task: Task) => (
                                    <tr key={task.id} className="border-b border-gray-800 last:border-0 hover:bg-gray-900/50 transition-colors">
                                        <td className="py-3 font-medium flex items-center gap-2">
                                            {task.name}
                                            {task.is_enabled && (
                                                <span className="relative flex h-3 w-3" title="Recording Active">
                                                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
                                                    <span className="relative inline-flex rounded-full h-3 w-3 bg-red-500"></span>
                                                </span>
                                            )}
                                        </td>
                                        <td className="py-3 text-blue-400 truncate max-w-[300px]">{task.target_url}</td>
                                        <td className="py-3 text-right flex justify-end gap-2">
                                            <div className="flex gap-2">
                                                {task.is_enabled ? (
                                                    <button
                                                        onClick={() => stopMutation.mutate(task.id)}
                                                        className="p-2 bg-red-900/30 text-red-500 rounded hover:bg-red-900/50 transition-colors"
                                                        title="Stop Recording"
                                                    >
                                                        <Square size={16} />
                                                    </button>
                                                ) : (
                                                    <div className="flex gap-2">
                                                        <button
                                                            onClick={() => startMutation.mutate(task.id)}
                                                            className="p-2 bg-green-900/30 text-green-500 rounded hover:bg-green-900/50 transition-colors"
                                                            title="Start Recording"
                                                        >
                                                            <Play size={16} />
                                                        </button>
                                                        <button
                                                            onClick={() => setInteractingTask(task)}
                                                            className="p-2 bg-blue-900/30 text-blue-500 rounded hover:bg-blue-900/50 transition-colors"
                                                            title="Interact (Remote Control)"
                                                        >
                                                            <MousePointer size={16} />
                                                        </button>
                                                    </div>
                                                )}
                                            </div>
                                            <button
                                                className="bg-gray-800 text-gray-400 hover:bg-gray-700 px-3 py-1 rounded flex items-center gap-1 transition-colors"
                                                onClick={() => setEditingTask(task)}
                                                title="Edit Settings"
                                            >
                                                <Settings size={14} />
                                            </button>
                                            <button
                                                className="bg-red-600/20 text-red-400 hover:bg-red-600/30 px-3 py-1 rounded flex items-center gap-1 transition-colors"
                                                onClick={() => {
                                                    if (confirm("Delete this task?")) {
                                                        axios.delete(`/api/tasks/${task.id}`)
                                                            .then(() => queryClient.invalidateQueries({ queryKey: ['tasks'] }))
                                                    }
                                                }}
                                            >
                                                Trash
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                ) : (
                    <div className="text-gray-500 text-sm py-4 text-center">No tasks defined. Create one to start recording.</div>
                )}
            </div>

            {/* Create Task Modal */}
            {isCreateModalOpen && (
                <TaskModal
                    title="New Recording Task"
                    onClose={() => setIsCreateModalOpen(false)}
                    onSubmit={(data) => createMutation.mutate(data)}
                    isPending={createMutation.isPending}
                />
            )}

            {editingTask && (
                <TaskModal
                    title="Edit Task"
                    initialData={editingTask}
                    onClose={() => setEditingTask(null)}
                    onSubmit={(data) => updateMutation.mutate({ ...data, id: editingTask.id })}
                    isPending={updateMutation.isPending}
                />
            )}

            {/* Interact Modal */}
            <InteractModal
                isOpen={!!interactingTask}
                onClose={() => setInteractingTask(null)}
                task={interactingTask ? { id: interactingTask.id, name: interactingTask.name } : null}
            />
        </div>
    )
}

function TaskModal({ title, onClose, onSubmit, isPending, initialData }: {
    title: string
    onClose: () => void,
    onSubmit: (data: any) => void,
    isPending: boolean,
    initialData?: Task
}) {
    const [name, setName] = React.useState(initialData?.name || '')
    const [url, setUrl] = React.useState(initialData?.target_url || '')
    const [fps, setFps] = React.useState((initialData as any)?.fps || 5)
    const [crf, setCrf] = React.useState((initialData as any)?.crf || 23)
    const [filenameTemplate, setFilenameTemplate] = React.useState(initialData?.filename_template || '')
    const [customCSS, setCustomCSS] = React.useState(initialData?.custom_css || '')
    const [timeOverlay, setTimeOverlay] = React.useState(initialData?.time_overlay || false)
    const [timeOverlayConfig, setTimeOverlayConfig] = React.useState(initialData?.time_overlay_config || 'bottom-right')
    const [previewImage, setPreviewImage] = React.useState<string | null>(null)
    const [isPreviewLoading, setIsPreviewLoading] = React.useState(false)
    const [previewError, setPreviewError] = React.useState<string | null>(null)

    const handlePreview = async () => {
        if (!url) return
        setIsPreviewLoading(true)
        setPreviewError(null)
        setPreviewImage(null)

        try {
            const res = await axios.post('/api/tasks/preview', {
                target_url: url,
                custom_css: customCSS
            }, { responseType: 'blob' })

            const imageUrl = URL.createObjectURL(res.data)
            setPreviewImage(imageUrl)
        } catch (err: any) {
            // Blob error reading is tricky, try to parse text if possible or just show generic
            setPreviewError("Failed to generate preview. Check URL accessibility.")
            console.error(err)
        } finally {
            setIsPreviewLoading(false)
        }
    }

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        onSubmit({
            name,
            target_url: url,
            filename_template: filenameTemplate,
            custom_css: customCSS,
            fps,
            crf,
            time_overlay: timeOverlay,
            time_overlay_config: timeOverlayConfig
        })
    }

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-gray-900 border border-gray-800 rounded-xl p-6 w-full max-w-md shadow-2xl overflow-y-auto max-h-[90vh]">
                <h3 className="text-xl font-bold mb-4">{title}</h3>
                <form onSubmit={handleSubmit} className="space-y-4">
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Task Name</label>
                        <input
                            type="text"
                            required
                            value={name}
                            onChange={e => setName(e.target.value)}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none"
                            placeholder="e.g. Grafana Dashboard"
                        />
                    </div>
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Target URL</label>
                        <input
                            type="url"
                            required
                            value={url}
                            onChange={e => setUrl(e.target.value)}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none"
                            placeholder="http://internal-dashboard:3000"
                        />
                    </div>
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Filename Prefix (Optional)</label>
                        <div className="flex items-center gap-2">
                            <input
                                type="text"
                                value={filenameTemplate}
                                onChange={e => setFilenameTemplate(e.target.value)}
                                className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none"
                                placeholder="e.g. my_dashboard"
                            />
                            <span className="text-gray-500 text-xs shrink-0 whitespace-nowrap">_YYYYMMDDHHMMSS.mkv</span>
                        </div>
                    </div>
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Frame Rate (FPS)</label>
                        <input
                            type="number"
                            min="1"
                            max="15"
                            required
                            value={fps}
                            onChange={e => setFps(Number(e.target.value))}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none"
                            placeholder="5"
                        />
                        <p className="text-xs text-gray-500 mt-1">Min: 1, Max: 15</p>
                        <p className="text-xs text-gray-500 mt-1">Recommended: 30 for video, 5-10 for simple dashboards. Max: 60.</p>
                    </div>
                    <div>
                        <label className="block text-sm text-gray-400 mb-1">Quality (CRF)</label>
                        <input
                            type="number"
                            min="0"
                            max="51"
                            required
                            value={crf}
                            onChange={e => setCrf(Number(e.target.value))}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none"
                            placeholder="23"
                        />
                        <p className="text-xs text-gray-500 mt-1">
                            0-51 (Lower value = Higher Quality & Larger File Size). <br />
                            Default: 23. Recommended: 18-28.
                        </p>
                    </div>
                    <div>
                        <div className="flex justify-between items-center mb-1">
                            <label className="block text-sm text-gray-400">Custom CSS (Optional)</label>
                            <button
                                type="button"
                                onClick={() => setCustomCSS("body { background-color: rgba(0, 0, 0, 0); margin: 0px auto; overflow: hidden; }")}
                                className="text-xs text-blue-400 hover:text-blue-300"
                            >
                                Insert OBS Default
                            </button>
                        </div>
                        <textarea
                            value={customCSS}
                            onChange={e => setCustomCSS(e.target.value)}
                            className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none min-h-[80px] text-xs font-mono mb-2"
                            placeholder="body { background-color: rgba(0, 0, 0, 0); margin: 0px auto; overflow: hidden; }"
                        />
                        <button
                            type="button"
                            onClick={handlePreview}
                            disabled={isPreviewLoading || !url}
                            className="text-xs bg-gray-800 hover:bg-gray-700 text-gray-300 px-3 py-1 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                        >
                            {isPreviewLoading ? 'Generating Preview...' : 'Preview CSS'}
                        </button>

                        {previewError && (
                            <div className="mt-2 text-red-500 text-xs">{previewError}</div>
                        )}

                        {previewImage && (
                            <div className="mt-2 border border-gray-700 rounded overflow-hidden">
                                <img
                                    src={previewImage}
                                    className="w-full h-auto object-contain"
                                    alt="CSS Preview"
                                />
                            </div>
                        )}
                    </div>

                    <div className="border-t border-gray-800 pt-4 mt-4">
                        <h4 className="text-sm font-semibold mb-3 text-gray-300">Overlays</h4>
                        <div className="flex flex-col gap-3">
                            <label className="flex items-center gap-2 cursor-pointer">
                                <input
                                    type="checkbox"
                                    checked={timeOverlay}
                                    onChange={e => setTimeOverlay(e.target.checked)}
                                    className="w-4 h-4 rounded border-gray-700 bg-gray-900 text-blue-600 focus:ring-blue-500 focus:ring-offset-gray-900"
                                />
                                <span className="text-sm text-gray-300">Show Time Overlay (NTP Synchronized)</span>
                            </label>

                            {timeOverlay && (
                                <div>
                                    <label className="block text-sm text-gray-400 mb-1">Overlay Position</label>
                                    <select
                                        value={timeOverlayConfig}
                                        onChange={e => setTimeOverlayConfig(e.target.value)}
                                        className="w-full bg-gray-950 border border-gray-800 rounded px-3 py-2 text-white focus:border-blue-500 outline-none text-sm"
                                    >
                                        <option value="top-left">Top Left</option>
                                        <option value="top-right">Top Right</option>
                                        <option value="bottom-left">Bottom Left</option>
                                        <option value="bottom-right">Bottom Right</option>
                                    </select>
                                </div>
                            )}
                        </div>
                    </div>

                    <div className="flex justify-end gap-3 mt-6">
                        <button
                            type="button"
                            onClick={onClose}
                            className="px-4 py-2 text-gray-300 hover:text-white transition-colors"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={isPending}
                            className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors flex items-center gap-2"
                        >
                            {isPending && <div className="animate-spin rounded-full h-4 w-4 border-2 border-white/20 border-t-white"></div>}
                            {isPending && <div className="animate-spin rounded-full h-4 w-4 border-2 border-white/20 border-t-white"></div>}
                            Save Task
                        </button>
                    </div>
                </form>
            </div>
        </div>
    )
}
