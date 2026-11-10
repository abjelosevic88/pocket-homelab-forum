// Building blocks every forum page shares: faces, forum and category marks,
// topic rows, the "how closely you follow" menu.
import React from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ago, fmtDateTime } from '../lib/format'
import { LEVEL, LEVELS, avatarURL, compact, eventParts, eventWhen, forumColor, topicPath } from '../lib/forum'
import type { CatMini, ForumMini, Level, Poster, Topic, TopicList } from '../lib/types'
import { useApp } from '../lib/app-context'
import { Icon } from './icons'
import { Avatar, Field, type MenuItem, MenuButton, Modal } from './ui'

export function Face({ name, color, avatarId, size = 28, pub = false, title }: { name: string; color?: string; avatarId?: number; size?: number; pub?: boolean; title?: string }) {
  if (avatarId) return <img src={avatarURL(avatarId, pub)} alt="" className="face-img" width={size} height={size} style={{ width: size, height: size }} title={title || name} />
  return <span title={title || name} className="face"><Avatar name={name.replace(/\s*\(guest\)$/, '')} size={size} color={color || undefined} /></span>
}

export function Faces({ list, max = 4, size = 24 }: { list: Poster[]; max?: number; size?: number }) {
  return (
    <span className="faces" aria-label={list.map(p => p.name).join(', ')}>
      {list.slice(0, max).map((p, i) => <Face key={p.id + ':' + i} name={p.name} color={p.color} size={size} />)}
    </span>
  )
}

export function ForumMark({ f, size = 28 }: { f: Pick<ForumMini, 'emoji' | 'color'>; size?: number }) {
  return <span className="forum-mark" style={{ width: size, height: size, fontSize: size * 0.55, background: `color-mix(in srgb, ${forumColor(f.color)} 16%, transparent)` }} aria-hidden="true">{f.emoji || '💬'}</span>
}

export function CatChip({ c, to }: { c?: CatMini | null; to?: string }) {
  if (!c) return null
  const inner = <><i className="cat-dot" style={{ background: c.color || 'var(--faint)' }} />{c.emoji && <span aria-hidden="true">{c.emoji}</span>}{c.name}</>
  return to ? <Link to={to} className="cat-chip" onClick={e => e.stopPropagation()}>{inner}</Link> : <span className="cat-chip">{inner}</span>
}

export function TagChips({ tags, forum }: { tags: string[]; forum?: string }) {
  if (!tags?.length) return null
  return <span className="tag-chips">{tags.map(t => forum
    ? <Link key={t} to={`/f/${forum}?tag=${encodeURIComponent(t)}`} className="tag-chip" onClick={e => e.stopPropagation()}>#{t}</Link>
    : <span key={t} className="tag-chip">#{t}</span>)}</span>
}

/** A topic's little marks: pinned, closed, solved, an event's date. */
export function TopicMarks({ t }: { t: Topic }) {
  return (
    <>
      {t.pinned > 0 && <Icon name="pin" size={13} className="t-mark pin" title={t.pinned === 2 ? 'Pinned on the front page' : 'Pinned in its category'} />}
      {t.closed && <Icon name="lock" size={13} className="t-mark" title="Closed" />}
      {t.archived && <Icon name="archive" size={13} className="t-mark" title="Archived" />}
      {t.unlisted && <Icon name="eye-off" size={13} className="t-mark" title="Unlisted: only reachable by its link" />}
      {t.kind === 'question' && (t.solved_post_id ? <span className="t-solved" title="Solved"><Icon name="check-circle" size={13} />Solved</span> : <Icon name="help" size={13} className="t-mark" title="A question waiting for an answer" />)}
      {t.kind === 'announcement' && <Icon name="alert" size={13} className="t-mark" title="Announcement" />}
    </>
  )
}

function EventBadge({ start }: { start: string }) {
  const p = eventParts(start)
  return <span className="ev-badge" aria-hidden="true"><b>{p.day}</b><small>{p.month}</small></span>
}

export function TopicRow({ t, forum, cat, showForum }: { t: Topic; forum?: ForumMini; cat?: CatMini; showForum?: boolean }) {
  const navigate = useNavigate()
  const { settings } = useApp()
  const compactRows = settings?.density === 'compact'
  const go = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('a,button')) return
    navigate(topicPath(t.id, t.visited && t.unread > 0 ? t.last_read + 1 : undefined))
  }
  const fresh = t.is_new || (t.visited && t.unread > 0)
  const href = topicPath(t.id, t.visited && t.unread > 0 ? t.last_read + 1 : undefined)
  return (
    <div className={`t-row ${compactRows ? 'compact' : ''} ${fresh ? 'fresh' : ''} ${t.level === 'muted' ? 'muted-row' : ''}`} onClick={go} role="listitem">
      {t.kind === 'event' && t.event_start ? <EventBadge start={t.event_start} /> : <Face name={t.posters[0]?.name || t.author_name} color={t.posters[0]?.color} size={compactRows ? 26 : 36} />}
      <div className="t-main">
        <div className="t-title-line">
          <TopicMarks t={t} />
          <Link to={href} className={`t-title ${t.visited && !t.unread ? 'read' : ''}`}>{t.title}</Link>
          {t.is_new && <span className="t-new" title="A topic you have not opened yet">new</span>}
          {t.visited && t.unread > 0 && <span className="t-unread" title={`${t.unread} new ${t.unread === 1 ? 'post' : 'posts'} since you last read it`}>{t.unread}</span>}
        </div>
        <div className="t-meta">
          {showForum && forum && <Link to={`/f/${forum.slug}`} className="t-forum" onClick={e => e.stopPropagation()}><span aria-hidden="true">{forum.emoji}</span>{forum.name}</Link>}
          {cat && forum && <CatChip c={cat} to={`/f/${forum.slug}/c/${cat.slug}`} />}
          <TagChips tags={t.tags} forum={forum?.slug} />
          {t.kind === 'event' && t.event_start && <span className="t-when"><Icon name="calendar" size={12} />{eventWhen(t.event_start)}{t.event_place && <> · {t.event_place}</>}</span>}
          {!compactRows && t.kind !== 'event' && t.excerpt && <span className="t-excerpt">{excerptOf(t.excerpt)}</span>}
        </div>
      </div>
      <div className="t-side">
        <Faces list={t.posters.slice(1)} max={3} size={22} />
        <span className="t-num" title={`${t.posts_n - 1} replies`}><Icon name="message" size={13} />{compact(Math.max(0, t.posts_n - 1))}</span>
        <span className="t-num hide-sm" title={`${t.views} views`}><Icon name="eye" size={13} />{compact(t.views)}</span>
        <span className="t-ago" title={t.last_post_at ? `Last post ${fmtDateTime(t.last_post_at)} by ${t.last_name}` : ''}>{ago(t.last_post_at || t.created_at)}</span>
      </div>
    </div>
  )
}

function excerptOf(md: string) {
  return md.replace(/\[quote=[^\]]*\][\s\S]*?\[\/quote\]/g, ' ').replace(/```[\s\S]*?```/g, ' ').replace(/!\[[^\]]*\]\([^)]*\)/g, '🖼').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').replace(/[`*_~#>|]+/g, '').replace(/\s+/g, ' ').trim().slice(0, 160)
}

export function TopicListView({ data, showForum, empty }: { data: TopicList | null; showForum?: boolean; empty?: React.ReactNode }) {
  if (!data) return null
  if (data.topics.length === 0) return <>{empty}</>
  return (
    <div className="t-list" role="list">
      {data.topics.map(t => <TopicRow key={t.id} t={t} showForum={showForum} forum={data.forums[t.forum_id]} cat={data.categories[t.category_id]} />)}
    </div>
  )
}

/** The bell-shaped "how closely do you follow this" button. */
export function LevelButton({ level, onChange, what, compact: small }: { level: Level | ''; onChange: (l: Level) => void; what: string; compact?: boolean }) {
  const cur = LEVEL[level || 'normal']
  const items: MenuItem[] = [{ heading: `Follow this ${what}` }, ...LEVELS.map(l => ({ label: <span className="lvl-opt"><b>{l.label}</b><small>{l.line}</small></span>, icon: l.icon, checked: (level || 'normal') === l.value, onSelect: () => onChange(l.value) }))]
  return (
    <MenuButton items={items} label={`Following: ${cur.label}`} className={`btn ${small ? 'icon-only' : ''} lvl-btn lvl-${level || 'normal'}`} align="right" width={300}>
      <Icon name={cur.icon} size={16} />{!small && cur.label}
    </MenuButton>
  )
}

export function Stat({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return <div className="stat"><span className="stat-l">{label}</span><span className="stat-v">{value}</span>{sub && <span className="stat-s small">{sub}</span>}</div>
}

/** "2 h ago" that keeps its hover with the exact time. */
export function When({ at }: { at: string }) {
  return <time dateTime={at} title={fmtDateTime(at)}>{ago(at)}</time>
}

/** A small dialog asking for one line of text (instead of the browser's prompt). */
export function AskText({ title, label, help, initial = '', placeholder, ok = 'Save', onClose, onDone }: { title: string; label: string; help?: string; initial?: string; placeholder?: string; ok?: string; onClose: () => void; onDone: (v: string) => void }) {
  const [v, setV] = React.useState(initial)
  return (
    <Modal title={title} onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => onDone(v)}>{ok}</button></>}>
      <Field label={label} help={help}><input value={v} onChange={e => setV(e.target.value)} placeholder={placeholder} autoFocus onKeyDown={e => { if (e.key === 'Enter') onDone(v) }} /></Field>
    </Modal>
  )
}
