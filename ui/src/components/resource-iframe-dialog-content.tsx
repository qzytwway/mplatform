import { useRef, type ReactNode } from 'react'
import { IconExternalLink, IconMaximize } from '@tabler/icons-react'

import { withSubPath } from '@/lib/subpath'
import { Button } from '@/components/ui/button'
import {
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export function ResourceIframeDialogContent({
  title,
  path,
}: {
  title: ReactNode
  path: string
}) {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const handleOpenCurrentPage = () => {
    const url = new URL(iframeRef.current!.contentWindow!.location.href)
    url.searchParams.delete('iframe')
    window.location.assign(url.toString())
  }

  return (
    <DialogContent className="!h-[calc(100dvh-1rem)] !max-w-[calc(100vw-1rem)] flex min-h-0 flex-col gap-0 p-0 md:!h-[80%] md:!max-w-[80%]">
      <DialogHeader className="flex flex-row items-center justify-between border-b px-4 py-3 pr-14">
        <DialogTitle>{title}</DialogTitle>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label="Open resource in current page"
            onClick={handleOpenCurrentPage}
          >
            <IconMaximize size={12} />
          </Button>
          <Button asChild variant="outline" size="icon">
            <a
              href={withSubPath(path)}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="Open resource in new tab"
            >
              <IconExternalLink size={12} />
            </a>
          </Button>
        </div>
      </DialogHeader>
      <iframe
        ref={iframeRef}
        src={`${withSubPath(path)}?iframe=true`}
        className="min-h-0 w-full flex-grow border-none"
      />
    </DialogContent>
  )
}
