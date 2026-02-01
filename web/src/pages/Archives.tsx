import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { FileVideo, Download, Trash2 } from 'lucide-react'

interface Archive {
    id: number
    task_name: string
    file_path: string
    start_time: string
    size: string
    status: string
}

export function Archives() {
    const queryClient = useQueryClient()
    const { data: archives, isLoading } = useQuery({
        queryKey: ['archives'],
        queryFn: async () => {
            const res = await axios.get('/api/archives')
            return res.data
        },
    })

    const deleteMutation = useMutation({
        mutationFn: async (id: number) => {
            await axios.delete(`/api/recordings/${id}`)
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['archives'] })
        },
        onError: (err: any) => {
            alert("Failed to delete: " + (err.response?.data?.error || err.message))
        }
    })

    return (
        <div>
            <div className="flex justify-between items-center mb-6">
                <h2 className="text-2xl font-bold">Archives</h2>
            </div>

            <div className="bg-gray-900 rounded-lg p-6 border border-gray-800">
                {isLoading ? (
                    <div className="text-gray-400 text-sm">Loading archives...</div>
                ) : archives && archives.length > 0 ? (
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                        {archives?.map((archive: Archive) => (
                            <div key={archive.id} className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden group hover:border-blue-500/50 transition-all">
                                <div className="p-4 flex items-start gap-4">
                                    <div className="bg-gray-800 p-2 rounded text-blue-400">
                                        <FileVideo size={24} />
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <div className="font-medium text-white truncate" title={archive.task_name}>
                                            {archive.task_name || 'Unknown Task'}
                                        </div>
                                        <div className="text-xs text-gray-500 mt-1">
                                            {new Date(archive.start_time).toLocaleString()}
                                        </div>
                                        <div className="text-xs text-gray-400 mt-1 font-mono">
                                            {archive.size}
                                        </div>
                                    </div>
                                </div>
                                <div className="bg-gray-800/50 px-4 py-2 flex justify-between items-center border-t border-gray-800">
                                    <span className={`text-xs px-2 py-0.5 rounded ${archive.status === 'COMPLETED' ? 'bg-green-500/10 text-green-500' :
                                        archive.status === 'RECORDING' ? 'bg-red-500/10 text-red-500 animate-pulse' :
                                            'bg-gray-700 text-gray-300'
                                        }`}>
                                        {archive.status}
                                    </span>
                                    <div className="flex items-center gap-3">
                                        <a
                                            href={`/recordings/${archive.file_path.split('/').pop()}`}
                                            download
                                            className="text-gray-400 hover:text-white transition-colors"
                                            title="Download"
                                            target="_blank"
                                            rel="noopener noreferrer"
                                        >
                                            <Download size={16} />
                                        </a>
                                        <button
                                            onClick={() => {
                                                if (confirm("Delete this recording permanently?")) {
                                                    deleteMutation.mutate(archive.id)
                                                }
                                            }}
                                            className="text-gray-400 hover:text-red-500 transition-colors"
                                            title="Delete"
                                        >
                                            <Trash2 size={16} />
                                        </button>
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                ) : (
                    <div className="text-gray-500 text-sm py-8 text-center bg-gray-900/50 rounded-lg border border-gray-800 border-dashed">
                        No recordings found. Start a task to generate archives.
                    </div>
                )}
            </div>
        </div>
    )
}
