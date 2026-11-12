import './App.css'

import { ReactNode, useLayoutEffect, useRef } from 'react'
import { IconServer } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link, Outlet, useLocation, useSearchParams } from 'react-router-dom'

import { AIChatbox, StandaloneAIChatbox } from './components/ai-chat/ai-chatbox'
import { AppSidebar } from './components/app-sidebar'
import { ErrorBoundary } from './components/error-boundary'
import { FloatingTerminal } from './components/floating-terminal'
import { GlobalSearch } from './components/global-search'
import {
  GlobalSearchProvider,
  useGlobalSearch,
} from './components/global-search-provider'
import { SiteHeader } from './components/site-header'
import { Button } from './components/ui/button'
import { SidebarInset, SidebarProvider } from './components/ui/sidebar'
import { Toaster } from './components/ui/sonner'
import { AIChatProvider } from './contexts/ai-chat-context'
import { useAuth } from './contexts/auth-context'
import { ClusterProvider } from './contexts/cluster-context'
import { TerminalProvider, useTerminal } from './contexts/terminal-context'
import { useCluster } from './hooks/use-cluster'

function ClusterGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { isLoading, error } = useCluster()

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="flex items-center space-x-2">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-muted border-t-foreground" />
          <span>{t('cluster.loading')}</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-destructive">
          <p>{t('cluster.error', { error: error.message })}</p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}

// Settings and cluster management stay reachable so an admin can add the
// first cluster.
const CLUSTER_FREE_PATHS = ['/settings', '/platform']

function NoCluster() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const isAdmin = user?.isAdmin() ?? false

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 text-center">
      <IconServer className="size-10 text-muted-foreground" stroke={1.5} />
      <div>
        <h2 className="text-base font-medium">{t('cluster.none.title')}</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {isAdmin
            ? t('cluster.none.description')
            : t('cluster.none.contactAdmin')}
        </p>
      </div>
      {isAdmin && (
        <Button asChild size="sm">
          <Link to="/settings?tab=clusters">
            {t('clusterManagement.actions.add')}
          </Link>
        </Button>
      )}
    </div>
  )
}

function AppContent() {
  const { isOpen, closeSearch } = useGlobalSearch()
  const { isOpen: isTerminalOpen } = useTerminal()
  const { clusters } = useCluster()
  const { pathname } = useLocation()
  const [searchParams] = useSearchParams()
  const isIframe = searchParams.get('iframe') === 'true'
  const insetRef = useRef<HTMLElement>(null)

  // Pages scroll inside the inset, which the router's ScrollRestoration does
  // not reach. Tabs kept in the query string leave the position alone.
  useLayoutEffect(() => {
    if (insetRef.current) insetRef.current.scrollTop = 0
  }, [pathname])
  const needsCluster =
    clusters.length === 0 &&
    !CLUSTER_FREE_PATHS.some(
      (path) => pathname === path || pathname.startsWith(`${path}/`)
    )

  if (isIframe) {
    return <Outlet />
  }

  return (
    <>
      <SidebarProvider>
        <AppSidebar variant="inset" />
        <SidebarInset
          ref={insetRef}
          className="h-dvh overflow-y-auto overscroll-none scrollbar-hide md:h-[calc(100dvh-1rem)]"
        >
          <SiteHeader />
          <div className="@container/main flex min-h-0 flex-1 flex-col">
            <div className="flex min-h-0 flex-1 flex-col gap-4 pt-4 md:gap-6">
              <div className="flex min-h-0 flex-1 flex-col px-4 lg:px-6">
                <ErrorBoundary>
                  {needsCluster ? <NoCluster /> : <Outlet />}
                </ErrorBoundary>
                {/* A page taller than the screen overflows these min-h-0
                    wrappers and leaves their bottom padding behind, so the gap
                    below it comes from this spacer instead. */}
                <div className="h-4 shrink-0" />
              </div>
            </div>
          </div>
        </SidebarInset>
      </SidebarProvider>
      {isOpen ? (
        <GlobalSearch open={isOpen} onOpenChange={closeSearch} />
      ) : null}
      {isTerminalOpen ? (
        <ErrorBoundary fallback={null}>
          <FloatingTerminal />
        </ErrorBoundary>
      ) : null}
      <ErrorBoundary fallback={null}>
        <AIChatbox />
      </ErrorBoundary>
      <Toaster />
    </>
  )
}

function AppProviders({ children }: { children: ReactNode }) {
  return (
    <TerminalProvider>
      <ClusterProvider>
        <GlobalSearchProvider>
          <AIChatProvider>{children}</AIChatProvider>
        </GlobalSearchProvider>
      </ClusterProvider>
    </TerminalProvider>
  )
}

function App() {
  return (
    <AppProviders>
      <ClusterGate>
        <AppContent />
      </ClusterGate>
    </AppProviders>
  )
}

export function StandaloneAIChatApp() {
  return (
    <AppProviders>
      <ClusterGate>
        <StandaloneAIChatbox />
      </ClusterGate>
    </AppProviders>
  )
}

export default App
