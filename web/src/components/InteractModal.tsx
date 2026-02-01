import { Dialog, Transition } from '@headlessui/react'
import { Fragment, useEffect, useRef, useState } from 'react'
import axios from 'axios'
import { X, Loader2, MousePointer } from 'lucide-react'

interface InteractModalProps {
    isOpen: boolean
    onClose: () => void
    task: { id: number, name: string } | null
}

type ConnectionStatus = 'IDLE' | 'CONNECTING' | 'CONNECTED' | 'ERROR' | 'RECONNECTING'

export function InteractModal({ isOpen, onClose, task }: InteractModalProps) {
    const [status, setStatus] = useState<ConnectionStatus>('IDLE')
    const [imgSrc, setImgSrc] = useState<string | null>(null)
    const [errorMsg, setErrorMsg] = useState<string | null>(null)
    const wsRef = useRef<WebSocket | null>(null)
    const imgRef = useRef<HTMLImageElement>(null)

    // Cleanup when modal closes or task changes
    useEffect(() => {
        if (!isOpen || !task) {
            if (wsRef.current) {
                wsRef.current.close()
                wsRef.current = null
            }
            setImgSrc(null)
            setStatus('IDLE')
            return
        }

        const connect = async () => {
            try {
                setStatus('CONNECTING')
                setErrorMsg(null)

                // 1. Get Ticket
                const ticketRes = await axios.post('/api/tickets', { task_id: task.id })
                const ticket = ticketRes.data.ticket

                // 2. Connect WS
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
                const wsUrl = `${protocol}//${window.location.host}/api/tasks/${task.id}/interact?ticket=${ticket}`

                const ws = new WebSocket(wsUrl)
                wsRef.current = ws

                ws.onopen = () => setStatus('CONNECTED')

                ws.onclose = (event) => {
                    if (event.code === 4001) {
                        setErrorMsg("Session expired. Please close and try again.")
                        setStatus('ERROR')
                    } else if (event.code === 4003) {
                        setErrorMsg("Access denied. You are not authorized.")
                        setStatus('ERROR')
                    } else if (!event.wasClean && status === 'CONNECTED') {
                        // Unexpected drop
                        setStatus('DISCONNECTED' as any) // Type hack or add DISCONNECTED? User asked for RECONNECTING/ERROR.
                        // Let's use ERROR for now with generic message, or RECONNECTING if we implemented retry.
                        // Plan: "Distinguish 'Connection Error' (Retryable) vs 'Auth Error'"
                        setErrorMsg("Connection lost.")
                        setStatus('ERROR')
                    } else {
                        setStatus('IDLE') // Clean close
                    }
                }

                ws.onerror = () => {
                    // onerror usually precedes onclose, let onclose handle final state if possible,
                    // but for connection failure start:
                    if (ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
                        setStatus('ERROR')
                        setErrorMsg("Failed to connect.")
                    }
                }

                ws.onmessage = (event) => {
                    if (event.data instanceof Blob) {
                        const url = URL.createObjectURL(event.data)
                        setImgSrc(prev => {
                            if (prev) URL.revokeObjectURL(prev)
                            return url
                        })
                    }
                }

            } catch (err: any) {
                console.error(err)
                setStatus('ERROR')
                setErrorMsg(err.response?.data?.error || err.message || "Failed to start session")
            }
        }

        connect()

        return () => {
            if (wsRef.current) {
                if (wsRef.current.readyState === WebSocket.OPEN) {
                    wsRef.current.send(JSON.stringify({ type: 'save' }))
                }
                wsRef.current.close()
            }
        }
    }, [isOpen, task])

    const sendEvent = (event: any) => {
        if (wsRef.current?.readyState === WebSocket.OPEN) {
            wsRef.current.send(JSON.stringify(event))
        }
    }

    const handleClick = (e: React.MouseEvent) => {
        if (!imgRef.current) return
        const rect = imgRef.current.getBoundingClientRect()
        const x = e.clientX - rect.left
        const y = e.clientY - rect.top
        sendEvent({ type: 'click', x, y })
    }

    const handleKeyDown = (e: React.KeyboardEvent) => {
        sendEvent({ type: 'key', key: e.key })
    }

    // Handle Esc key explicitly is good, but Dialog handles it for closing.
    // We just ensure we don't block it.

    return (
        <Transition appear show={isOpen} as={Fragment}>
            <Dialog as="div" className="relative z-50" onClose={onClose}>
                <Transition.Child
                    as={Fragment}
                    enter="ease-out duration-300"
                    enterFrom="opacity-0"
                    enterTo="opacity-100"
                    leave="ease-in duration-200"
                    leaveFrom="opacity-100"
                    leaveTo="opacity-0"
                >
                    <div className="fixed inset-0 bg-black/80" />
                </Transition.Child>

                <div className="fixed inset-0 overflow-y-auto">
                    <div className="flex min-h-full items-center justify-center p-4 text-center">
                        <Transition.Child
                            as={Fragment}
                            enter="ease-out duration-300"
                            enterFrom="opacity-0 scale-95"
                            enterTo="opacity-100 scale-100"
                            leave="ease-in duration-200"
                            leaveFrom="opacity-100 scale-100"
                            leaveTo="opacity-0 scale-95"
                        >
                            <Dialog.Panel className="w-full max-w-[1920px] transform overflow-hidden rounded-2xl bg-gray-900 p-6 text-left align-middle shadow-xl transition-all border border-gray-800">
                                <div className="flex justify-between items-center mb-4">
                                    <Dialog.Title as="h3" className="text-lg font-medium leading-6 text-white flex items-center gap-2">
                                        <MousePointer className="w-5 h-5 text-blue-400" />
                                        Remote Control: {task?.name}
                                    </Dialog.Title>
                                    <div className="flex items-center gap-4">
                                        <div className="text-sm">
                                            {status === 'CONNECTING' && <span className="text-blue-400 flex items-center gap-1"><Loader2 className="w-3 h-3 animate-spin" /> Connecting...</span>}
                                            {status === 'CONNECTED' && <span className="text-green-400 flex items-center gap-1"><div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" /> Live</span>}
                                            {status === 'IDLE' && <span className="text-gray-500">Disconnected</span>}
                                            {status === 'ERROR' && <span className="text-red-500 font-bold">{errorMsg || 'Error'}</span>}
                                        </div>
                                        <button
                                            onClick={onClose}
                                            className="text-gray-400 hover:text-white transition-colors"
                                        >
                                            <X className="w-5 h-5" />
                                        </button>
                                    </div>
                                </div>

                                <div
                                    className="relative bg-black rounded-lg overflow-hidden flex items-center justify-center min-h-[400px] outline-none ring-1 ring-gray-800 focus:ring-blue-500/50 transition-all"
                                    tabIndex={0}
                                    onKeyDown={handleKeyDown}
                                >
                                    {imgSrc ? (
                                        <img
                                            ref={imgRef}
                                            src={imgSrc}
                                            alt="Remote View"
                                            className="w-full h-auto cursor-crosshair"
                                            onMouseDown={handleClick}
                                            draggable={false}
                                        />
                                    ) : (
                                        <div className="text-gray-500 flex flex-col items-center gap-2">
                                            {status === 'ERROR' ? (
                                                <div className="text-red-400 flex flex-col gap-2 items-center">
                                                    <Loader2 className="w-8 h-8 text-red-500" />
                                                    <span>{errorMsg}</span>
                                                </div>
                                            ) : (
                                                <>
                                                    <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
                                                    <span>Waiting for video stream...</span>
                                                </>
                                            )}
                                        </div>
                                    )}

                                    {/* Overlay hints */}
                                    {status === 'CONNECTED' && (
                                        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 bg-black/60 text-white text-xs px-3 py-1 rounded-full pointer-events-none backdrop-blur-sm border border-white/10">
                                            Click to interact • Type to send keys
                                        </div>
                                    )}
                                </div>
                            </Dialog.Panel>
                        </Transition.Child>
                    </div>
                </div>
            </Dialog>
        </Transition>
    )
}
