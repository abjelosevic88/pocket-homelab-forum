// Left column: new topic (and new forum), the sections with what waits in
// each, and the person's forums with their new and unread topics.
import { useCallback, useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { useLive, useUnread } from '../lib/live'
import type { Home } from '../lib/types'
import { Icon } from './icons'
import { type CreateKind } from './Layout'
import { ForumMark } from './forum'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { unread: inbox, setUnread } = useUnread()
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const [home, setHome] = useState<Home | null>(null)
  const load = useCallback(() => { api.get<Home>(`${V1}/home`).then(h => { setHome(h); setUnread(h.inbox) }).catch(() => {}) }, [setUnread])
  useEffect(() => { load() }, [load, loc.pathname, loc.search])
  useOnChange(load)
  // new posts change the counts: catch up at most every few seconds
  const [stale, setStale] = useState(0)
  useLive(e => { if (e.type === 'post' || e.type === 'topic' || e.type === 'review') setStale(n => n + 1) })
  useEffect(() => { if (!stale) return; const t = window.setTimeout(load, 4000); return () => window.clearTimeout(t) }, [stale, load])

  const title = settings?.title || status?.title || 'Pocket Forum'
  const path = loc.pathname
  const tab = new URLSearchParams(loc.search).get('tab')
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; hint?: string; strong?: boolean }[] = [
    { to: '/', label: 'Home', icon: 'home', active: path === '/' && (!tab || tab === 'latest' || tab === 'top'), hint: 'The latest topics of all your forums' },
    { to: '/?tab=unread', label: 'Unread', icon: 'eye', active: path === '/' && tab === 'unread', count: home?.unread || undefined, hint: 'Topics with posts you have not read', strong: true },
    { to: '/?tab=new', label: 'New', icon: 'sparkles', active: path === '/' && tab === 'new', count: home?.new || undefined, hint: 'Topics you have not opened yet' },
    { to: '/inbox', label: 'Notifications', icon: 'bell', active: path.startsWith('/inbox'), count: inbox || undefined, hint: 'Mentions, replies and more', strong: true },
    { to: '/bookmarks', label: 'Bookmarks', icon: 'bookmark', active: path.startsWith('/bookmarks'), count: (home?.bookmarks || 0) + (home?.drafts || 0) || undefined, hint: 'Saved posts and unfinished drafts' },
    { to: '/events', label: 'Events', icon: 'calendar', active: path.startsWith('/events') },
    { to: '/search', label: 'Search', icon: 'search', active: path.startsWith('/search') },
  ]
  if (home?.moderator) nav.push({ to: '/review', label: 'Review', icon: 'shield', active: path.startsWith('/review'), count: home.review || undefined, hint: 'Held posts, flags and people asking to join', strong: true })
  const createMenu = [
    { label: 'New topic', icon: 'message-plus', hint: 'N', onSelect: () => onCreate('topic') },
    { label: 'New forum', icon: 'forum', onSelect: () => onCreate('forum') },
  ]
  const active = (slug: string) => path === `/f/${slug}` || path.startsWith(`/f/${slug}/`)
  return (
    <aside className="sidebar" aria-label="Pocket Forum">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Forum' ? title : <>Pocket <b>Forum</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('topic')} title="New topic (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />New topic
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createMenu} onClose={() => setMenu(null)} width={220} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${n.strong ? 'strong' : ''}`} title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      <section className="sb-sec">
        <h4 className="sb-h"><Link to="/forums" onClick={onNavigate} className="sb-h-link">Forums</Link>{(home?.discover || 0) > 0 && <Link to="/forums" onClick={onNavigate} className="sb-h-more" title="Forums you can join">{home!.discover} to join</Link>}</h4>
        {home?.forums.map(f => (
          <button key={f.id} type="button" className={`sb-row sb-list ${active(f.slug) ? 'active' : ''} ${f.archived ? 'dim' : ''}`} onClick={() => { onNavigate(); navigate(`/f/${f.slug}`) }} title={f.about || f.name}>
            <ForumMark f={f} size={22} />
            <span className="grow ellipsis">{f.name}</span>
            {f.status === 'pending' ? <span className="muted small">asked</span>
              : (f.pending || 0) > 0 ? <span className="sb-count strong" title="People asking to join">{f.pending}</span>
                : f.unread > 0 ? <span className="sb-count" title={`${f.unread} unread`}>{f.unread}</span>
                  : f.new > 0 ? <span className="sb-dot" title={`${f.new} new`} /> : null}
          </button>
        ))}
        {home && home.forums.length === 0 && <button type="button" className="sb-row sb-list" onClick={() => { onNavigate(); navigate('/forums') }}><Icon name="plus" size={15} className="sb-list-ico" /><span className="grow">Make or join a forum</span></button>}
      </section>

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pfrm:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
