// The person's own lists: notifications, bookmarks and drafts, events,
// search (with "ask the forum").
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { useLive, useUnread } from '../lib/live'
import { ago, fmtDateTime } from '../lib/format'
import { eventParts, eventWhen } from '../lib/forum'
import type { Bookmark, Draft, ForumBrief, ForumMini, Hit, Notification, TopicList } from '../lib/types'
import { Icon } from '../components/icons'
import { Face, TopicListView } from '../components/forum'
import { Markdown, Snippet } from '../components/Markdown'
import { useCrumb } from '../components/Layout'
import { Empty, Loading, Segmented, Spinner, Switch, Tabs, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'

const KIND_ICON: Record<string, string> = {
  mention: 'at', quote: 'quote', reply: 'reply', watching: 'message', new_topic: 'message-plus', reaction: 'heart', solved: 'check-circle', join_request: 'users',
  joined: 'check', welcome: 'sparkles', flag: 'flag', pending: 'clock', moderation: 'shield', reminder: 'bookmark',
}

function dayLabel(iso: string) {
  const d = new Date(iso), t = new Date()
  const y = new Date(); y.setDate(t.getDate() - 1)
  if (d.toDateString() === t.toDateString()) return 'Today'
  if (d.toDateString() === y.toDateString()) return 'Yesterday'
  return d.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' })
}

export function Inbox() {
  useCrumb(undefined)
  const navigate = useNavigate()
  const { setUnread } = useUnread()
  const [only, setOnly] = useState(false)
  const [list, setList] = useState<Notification[] | null>(null)
  const load = useCallback(() => {
    api.get<{ notifications: Notification[]; unread: number }>(`${V1}/notifications?limit=150${only ? '&unread=1' : ''}`).then(r => { setList(r.notifications); setUnread(r.unread) }).catch(() => setList([]))
  }, [only, setUnread])
  useEffect(() => { load() }, [load])
  useLive(e => { if (e.type === 'notification') load() })
  const read = async (ids: number[]) => {
    const r = await api.post<{ unread: number }>(`${V1}/notifications/read`, { ids })
    setUnread(r.unread)
    setList(l => l ? l.map(n => !ids.length || ids.includes(n.id) ? { ...n, read_at: n.read_at || new Date().toISOString() } : n) : l)
    announceChange()
  }
  const open = (n: Notification) => { if (!n.read_at) read([n.id]); navigate(n.path) }
  let last = ''
  return (
    <div className="page narrow">
      <div className="page-head"><h1 className="page-title grow">Notifications</h1>
        <label className="check-row small"><Switch size="sm" checked={only} onChange={setOnly} label="Only unread" />Only unread</label>
        <button type="button" className="btn" onClick={() => read([])} disabled={!list?.some(n => !n.read_at)}><Icon name="check" size={15} />Mark all read</button></div>
      <div className="card flush-card">
        {!list ? <Loading /> : list.length === 0 ? <Empty title={only ? 'Nothing unread' : 'Nothing yet'} icon="bell">When someone mentions you, answers you or posts where you follow, it shows up here.</Empty> : (
          <div className="rows">
            {list.map(n => {
              const d = dayLabel(n.created_at)
              const head = d !== last ? <div className="day-head" key={'d' + n.id}>{d}</div> : null
              last = d
              return [head, (
                <button key={n.id} type="button" className={`row notif ${n.read_at ? 'read' : ''}`} onClick={() => open(n)}>
                  <span className={`notif-ico k-${n.kind}`}><Icon name={KIND_ICON[n.kind] || 'bell'} size={15} /></span>
                  <span className="grow"><span className="notif-text">{n.text}</span>
                    {n.excerpt && <span className="muted small ellipsis">{n.excerpt}</span>}
                    <span className="muted small">{n.forum_emoji} {n.forum_name}{n.forum_name ? ' · ' : ''}{ago(n.created_at)}</span></span>
                  {!n.read_at && <span className="sb-dot" aria-label="unread" />}
                </button>
              )]
            })}
          </div>
        )}
      </div>
    </div>
  )
}

export function Bookmarks() {
  const { toast } = useToast()
  const [tab, setTab] = useState<'saved' | 'drafts'>('saved')
  const [list, setList] = useState<Bookmark[] | null>(null)
  const [drafts, setDrafts] = useState<Draft[] | null>(null)
  const [forums, setForums] = useState<ForumBrief[]>([])
  const load = useCallback(() => {
    api.get<Bookmark[]>(`${V1}/bookmarks`).then(setList).catch(() => setList([]))
    api.get<Draft[]>(`${V1}/drafts`).then(setDrafts).catch(() => setDrafts([]))
    api.get<{ forums: ForumBrief[] }>(`${V1}/forums`).then(r => setForums(r.forums)).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  const remove = async (b: Bookmark) => { try { await api.del(`${V1}/posts/${b.post_id}/bookmark`); load(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const dropDraft = async (d: Draft) => { try { await api.del(`${V1}/drafts/${d.key}`); load(); announceChange() } catch (e) { toast(errMsg(e), 'err') } }
  const draftLink = (d: Draft) => {
    const [kind, id] = d.key.split(':')
    if (kind === 'reply') return `/t/${id}`
    if (kind === 'new') { const f = forums.find(x => String(x.id) === id); return `/new${f ? `?forum=${f.slug}` : ''}` }
    return '/'
  }
  return (
    <div className="page narrow">
      <h1 className="page-title">Bookmarks</h1>
      <Tabs tabs={[{ id: 'saved', label: `Saved posts${list?.length ? ` (${list.length})` : ''}` }, { id: 'drafts', label: `Drafts${drafts?.length ? ` (${drafts.length})` : ''}` }]} value={tab} onChange={setTab} />
      <div className="card flush-card">
        {tab === 'saved' ? (!list ? <Loading /> : list.length === 0 ? <Empty title="No bookmarks" icon="bookmark">Bookmark a post from its ••• menu to keep it here, with a note and a reminder if you like.</Empty> : (
          <div className="rows">
            {list.map(b => (
              <div key={b.id} className="row bm-row">
                <Link to={`/t/${b.topic_id}/${b.number}`} className="grow bm-link">
                  <b>{b.topic_title}</b>
                  {b.note && <span className="bm-note"><Icon name="edit" size={12} /> {b.note}</span>}
                  <span className="muted small ellipsis">{b.author_name}: {b.excerpt}</span>
                  <span className="muted small">{b.forum_emoji} {b.forum_name}{b.remind_at && <> · <Icon name="bell" size={11} /> {b.reminded ? 'reminded' : 'reminder'} {fmtDateTime(b.remind_at)}</>}</span>
                </Link>
                <button type="button" className="btn ghost icon-only" aria-label="Remove the bookmark" title="Remove" onClick={() => remove(b)}><Icon name="x" size={15} /></button>
              </div>
            ))}
          </div>
        )) : (!drafts ? <Loading /> : drafts.length === 0 ? <Empty title="No drafts" icon="edit">What you start writing is kept here until you post it.</Empty> : (
          <div className="rows">
            {drafts.map(d => (
              <div key={d.key} className="row">
                <Link to={draftLink(d)} className="grow bm-link"><b>{d.data.title || (d.key.startsWith('reply') ? 'A reply' : 'A new topic')}</b><span className="muted small ellipsis">{(d.data.body || '').slice(0, 160) || '—'}</span><span className="muted small">{ago(d.updated_at)}</span></Link>
                <button type="button" className="btn ghost icon-only" aria-label="Throw the draft away" title="Throw away" onClick={() => dropDraft(d)}><Icon name="trash" size={15} /></button>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

export function Events() {
  const { toast } = useToast()
  const [data, setData] = useState<TopicList | null>(null)
  const load = useCallback(() => { api.get<TopicList>(`${V1}/events`).then(setData).catch(() => setData({ topics: [], forums: {}, categories: {}, more: false, mine: {} })) }, [])
  useEffect(() => { load() }, [load])
  useLive(e => { if (e.type === 'rsvp' || e.type === 'topic') load() })
  const answer = async (id: number, status: string) => { try { await api.post(`${V1}/topics/${id}/rsvp`, { status }); load() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="page narrow">
      <div className="page-head"><div className="grow"><h1 className="page-title">Events</h1><p className="muted small">Coming up in your forums. Say whether you come; the people organising see it.</p></div></div>
      {!data ? <Loading /> : data.topics.length === 0 ? <div className="card"><Empty title="Nothing planned" icon="calendar">Start a topic and pick “Event” to plan a meeting, a match or a night out.</Empty></div> : (
        <div className="stack">
          {data.topics.map(t => {
            const p = eventParts(t.event_start)
            const f = data.forums[t.forum_id]
            const mine = data.mine?.[t.id] || ''
            return (
              <div key={t.id} className="card event-row">
                <span className="ev-badge lg"><b>{p.day}</b><small>{p.month}</small></span>
                <Link to={`/t/${t.id}`} className="grow ev-link"><b>{t.title}</b><span className="muted small">{eventWhen(t.event_start, t.event_end)}{t.event_place ? ` · ${t.event_place}` : ''}</span><span className="muted small">{f?.emoji} {f?.name} · {Math.max(0, t.posts_n - 1)} replies</span></Link>
                <Segmented size="sm" value={mine || null} onChange={v => answer(t.id, v === mine ? '' : v)} options={[{ value: 'going', label: 'Going' }, { value: 'maybe', label: 'Maybe' }, { value: 'no', label: "Can't" }]} label="Are you coming?" />
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export function Search() {
  const [sp, setSp] = useSearchParams()
  const q = sp.get('q') || ''
  const forum = sp.get('forum') || ''
  const solved = sp.get('solved') || ''
  const titles = sp.get('titles') === '1'
  const [text, setText] = useState(q)
  const [res, setRes] = useState<{ hits: Hit[]; forums: Record<string, ForumMini> } | null>(null)
  const [busy, setBusy] = useState(false)
  const [forums, setForums] = useState<ForumBrief[]>([])
  const [ask, setAsk] = useState<{ answer: string; sources: { n: number; topic_id: number; title: string; solved: boolean }[] } | null>(null)
  const [askBusy, setAskBusy] = useState(false)
  const { toast } = useToast()
  useEffect(() => { api.get<{ forums: ForumBrief[] }>(`${V1}/forums`).then(r => setForums(r.forums.filter(f => f.status === 'active'))).catch(() => {}) }, [])
  useEffect(() => {
    setText(q)
    if (q.trim().length < 2) { setRes(null); return }
    setBusy(true)
    const u = new URLSearchParams({ q, limit: '60' })
    if (forum) u.set('forum', forum)
    if (solved) u.set('solved', solved)
    if (titles) u.set('titles', '1')
    api.get<{ hits: Hit[]; forums: Record<string, ForumMini> }>(`${V1}/search?${u}`).then(setRes).catch(() => setRes({ hits: [], forums: {} })).finally(() => setBusy(false))
  }, [q, forum, solved, titles])
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }
  const askForum = async () => {
    setAskBusy(true)
    try { setAsk(await api.post(`${V1}/ai/ask`, { forum, question: q })) } catch (e) { toast(errMsg(e), 'err') } finally { setAskBusy(false) }
  }
  return (
    <div className="page narrow">
      <h1 className="page-title">Search</h1>
      <form className="search-box" onSubmit={e => { e.preventDefault(); set('q', text.trim()); setAsk(null) }}>
        <Icon name="search" size={18} /><input value={text} onChange={e => setText(e.target.value)} placeholder="Words from a title or a post…" autoFocus aria-label="Search" />
        <button type="submit" className="btn primary">Search</button>
      </form>
      <div className="toolbar row wrap">
        {forums.length > 1 && <div className="tb-forum"><Select ariaLabel="Forum" value={forum} onChange={v => set('forum', v)} options={[{ value: '', label: 'All my forums' }, ...forums.map(f => ({ value: f.slug, label: `${f.emoji} ${f.name}` }))]} /></div>}
        <Segmented size="sm" value={solved} onChange={v => set('solved', v)} options={[{ value: '', label: 'Everything' }, { value: 'yes', label: 'Solved' }, { value: 'no', label: 'Open questions' }]} />
        <label className="check-row small"><Switch size="sm" checked={titles} onChange={v => set('titles', v ? '1' : '')} label="Titles only" />Titles only</label>
      </div>
      {q && <section className="card pad-card ai-card">
        {!ask ? <div className="row"><span className="grow muted"><Icon name="sparkles" size={15} /> Ask the forum: the AI answers from what has been written, with the topics it used.</span><button type="button" className="btn" disabled={askBusy} onClick={askForum}>{askBusy ? <Spinner size={14} /> : <Icon name="sparkles" size={15} />}Ask</button></div> : <>
          <b><Icon name="sparkles" size={15} /> From your forums</b>
          <Markdown text={ask.answer} />
          {ask.sources.length > 0 && <div className="chips">{ask.sources.map(s => <Link key={s.n} to={`/t/${s.topic_id}`} className="chip">[{s.n}] {s.title}{s.solved && <Icon name="check-circle" size={12} className="ok" />}</Link>)}</div>}
          <p className="muted small">Written by AI — open the topics to check.</p>
        </>}
      </section>}
      {busy && !res ? <Loading /> : res && (res.hits.length === 0 ? <div className="card"><Empty title="Nothing found" icon="search">Try fewer or other words — or ask it as a new topic.</Empty></div> : (
        <div className="card flush-card"><div className="rows">
          {res.hits.map(h => {
            const f = res.forums[h.forum_id]
            return (
              <Link key={h.post_id} to={h.number > 1 ? `/t/${h.topic_id}/${h.number}` : `/t/${h.topic_id}`} className="row hit">
                <span className="grow"><b>{h.solved && <Icon name="check-circle" size={13} className="ok" />} {h.title}{h.number > 1 && <span className="muted small"> · #{h.number}</span>}</b>
                  <span className="hit-snip"><Snippet text={h.snippet} /></span>
                  <span className="muted small">{h.author_name} · {ago(h.created_at)} · {f?.emoji} {f?.name}</span></span>
              </Link>
            )
          })}
        </div></div>
      ))}
    </div>
  )
}

export function Mini({ name }: { name: string }) { return <Face name={name} size={20} /> }
export { TopicListView }
