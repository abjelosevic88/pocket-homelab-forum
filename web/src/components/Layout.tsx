// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'
import { useUnread } from '../lib/live'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pfrm-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string, to?: string) => void>(() => {})
export function useCrumb(title: string | undefined, parent?: string) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || '', parent); return () => set('') }, [title, parent, set])
}

export type CreateKind = 'topic' | 'forum'

/** Start something from anywhere (a page's button). */
export const openCreate = (k: CreateKind) => window.dispatchEvent(new CustomEvent('pfrm:create', { detail: { kind: k } }))

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState<{ t: string; to?: string }>({ t: '' })
  const setCrumb = useCallback((t: string, to?: string) => setCrumbRaw({ t, to }), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)

  const create = (k: CreateKind) => {
    closeDrawer()
    if (k === 'forum') { navigate('/forums?new=1'); return }
    // a new topic starts in the forum (and category) on screen
    const m = /^\/f\/([a-z0-9-]+)(?:\/c\/([a-z0-9-]+))?/.exec(loc.pathname)
    navigate(m ? `/new?forum=${m[1]}${m[2] ? `&category=${m[2]}` : ''}` : '/new')
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => createRef.current((e as CustomEvent<{ kind: CreateKind }>).detail.kind)
    window.addEventListener('pfrm:shortcuts', open)
    window.addEventListener('pfrm:create', make)
    return () => { window.removeEventListener('pfrm:shortcuts', open); window.removeEventListener('pfrm:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      if (/^\/t\//.test(loc.pathname) && 'jklsrR'.includes(e.key)) return
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': e.preventDefault(); createRef.current('topic'); break
        case 'h': go('/'); break
        case 'u': go('/?tab=unread'); break
        case 'i': go('/inbox'); break
        case 'b': go('/bookmarks'); break
        case 'e': go('/events'); break
        case 'f': go('/forums'); break
        case 'r': go('/review'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'new', label: 'New topic', icon: 'message-plus', hint: 'N', keywords: 'write post start ask question event poll', run: () => create('topic') },
    { id: 'forum', label: 'New forum', icon: 'forum', keywords: 'create community club family team', run: () => create('forum') },
    { id: 'home', label: 'Home: latest topics', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'unread', label: 'Unread topics', icon: 'eye', hint: 'U', keywords: 'new posts', run: () => navigate('/?tab=unread') },
    { id: 'newt', label: 'New topics', icon: 'sparkles', keywords: 'fresh', run: () => navigate('/?tab=new') },
    { id: 'inbox', label: 'Notifications', icon: 'bell', hint: 'I', keywords: 'mentions replies inbox', run: () => navigate('/inbox') },
    { id: 'bookmarks', label: 'Bookmarks and drafts', icon: 'bookmark', hint: 'B', keywords: 'saved later reminder', run: () => navigate('/bookmarks') },
    { id: 'events', label: 'Events', icon: 'calendar', hint: 'E', keywords: 'meetings dates going', run: () => navigate('/events') },
    { id: 'forums', label: 'Forums', icon: 'forum', hint: 'F', keywords: 'discover join communities', run: () => navigate('/forums') },
    { id: 'search', label: 'Search everything', icon: 'search', keywords: 'find ask ai', run: () => navigate('/search') },
    { id: 'review', label: 'Review: held posts, flags, people asking to join', icon: 'shield', hint: 'R', keywords: 'moderate moderation queue', run: () => navigate('/review') },
    { id: 'me', label: 'My profile', icon: 'user', keywords: 'badges posts', run: () => navigate('/u/me') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'chat email contacts calendar tasks projects notes files photos learn blog bookshelf places translate core', run: () => navigate('/settings/apps') },
    { id: 'notif', label: 'Settings: notifications and the summary email', icon: 'bell', keywords: 'push digest email', run: () => navigate('/settings/notifications') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export and import', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner"><Outlet /></div></main>
        </div>
        {mobile && (/^\/(|events|bookmarks)$/.test(loc.pathname) || /^\/f\/[a-z0-9-]+(\/c\/[a-z0-9-]+)?$/.test(loc.pathname)) && <button type="button" className="fab" aria-label="New topic" title="New topic" onClick={() => create('topic')}><Icon name="plus" size={26} strokeWidth={2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = {
  '': ['Home', '/'], f: ['Forums', '/forums'], forums: ['Forums', '/forums'], t: ['Topic', '/'], new: ['New topic', '/'], inbox: ['Notifications', '/inbox'],
  bookmarks: ['Bookmarks', '/bookmarks'], events: ['Events', '/events'], search: ['Search', '/search'], review: ['Review', '/review'], u: ['People', '/'],
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: { t: string; to?: string } }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'My page', icon: 'user', onSelect: () => navigate('/u/me') },
    { label: 'Profile and handle', icon: 'at', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pfrm:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb.t ? <><button type="button" className="crumb" onClick={() => navigate(crumb.to || sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb.t}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <Bell />
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}

function Bell() {
  const { unread } = useUnread()
  const navigate = useNavigate()
  return (
    <button type="button" className="btn ghost icon-only bell-btn" onClick={() => navigate('/inbox')} aria-label={unread ? `Notifications: ${unread} unread` : 'Notifications'} title="Notifications (I)">
      <Icon name="bell" size={18} />{unread > 0 && <span className="bell-n">{unread > 99 ? '99+' : unread}</span>}
    </button>
  )
}
