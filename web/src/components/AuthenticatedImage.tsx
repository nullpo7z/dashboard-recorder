import { useState, useEffect } from 'react'
import axios from 'axios'
import { AlertCircle, Loader2 } from 'lucide-react'

interface AuthenticatedImageProps {
    src: string
    alt: string
    className?: string
}

export function AuthenticatedImage({ src, alt, className }: AuthenticatedImageProps) {
    const [displayedSrc, setDisplayedSrc] = useState<string | null>(null)
    const [error, setError] = useState(false)

    // Fetch new image
    useEffect(() => {
        let active = true
        // We do NOT reset displayedSrc here, to keep showing the old image while loading
        setError(false)

        axios.get(src, { responseType: 'blob' })
            .then(res => {
                if (active) {
                    const newUrl = URL.createObjectURL(res.data)
                    setDisplayedSrc(newUrl)
                }
            })
            .catch(() => {
                if (active) {
                    setError(true)
                }
            })

        return () => {
            active = false
        }
    }, [src])

    // Cleanup previous image URL only AFTER the render cycle has completed with the new URL.
    // React runs effect cleanups after the new render is committed.
    useEffect(() => {
        return () => {
            if (displayedSrc) {
                URL.revokeObjectURL(displayedSrc)
            }
        }
    }, [displayedSrc])

    return (
        <div className={`relative ${className}`}>
            {displayedSrc ? (
                <img
                    src={displayedSrc}
                    alt={alt}
                    className="w-full h-full object-contain"
                />
            ) : (
                <div className="flex items-center justify-center w-full h-full bg-gray-800 text-gray-500">
                    {error ? <AlertCircle className="w-6 h-6" /> : <Loader2 className="w-6 h-6 animate-spin" />}
                </div>
            )}
        </div>
    )
}
