// A forum: its latest topics (or one category's), the categories, top
// topics, open questions, events, the people and the about page.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useLive } from '../lib/live'
import { JOIN, VISIBILITY, forumColor } from '../lib/forum'
import { fmtDate } from '../lib/format'
import type { Category, ForumDoc, Level, Member, TopicList } from '../lib/types'
import { announceChange } from '../lib/data'
import { Icon } from '../components/icons'
import { Face, ForumMark, LevelButton, Stat, TopicListView, When } from '../components/forum'
import { Markdown } from '../components/Markdown'
import { InviteModal } from '../components/Invite'
import { useCrumb } from '../components/Layout'
import { CopyField, Empty, Loading, Modal, Spinner, Tabs, errMsg, useToast, type MenuItem, MenuButton } from '../components/ui'

type Tab = 'latest' | 'categories' | 'top' | 'unsolved' | 'events' | 'members' | 'about'

export default function ForumPage() {
  const { slug = '', cat: catSlug } = useParams()
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [doc, setDoc] = useState<ForumDoc | null>(null)
  const [err, setErr] = useState('')
  const [invite, setInvite] = useState(false)
  const [asking, setAsking] = useState(false)
  const tab = (sp.get('tab') as Tab) || (catSlug ? 'latest' : 'latest')
  const tag = sp.get('tag') || ''
  const load = useCallback(() => {
    api.get<ForumDoc>(`${V1}/forums/${slug}`).then(d => { setDoc(d); setErr('') }).catch(e => setErr(errMsg(e)))
  }, [slug])
  useEffect(() => { setDoc(null); load() }, [load])
  useLive(e => { if (e.type === 'forum' && e.forum_id === doc?.forum.id) load() })
  const cat = useMemo(() => doc?.categories.find(c => c.slug === catSlug) || null, [doc, catSlug])
  useCrumb(doc ? (cat ? `${doc.forum.name} › ${cat.name}` : doc.forum.name) : undefined, '/forums')

  if (err) return <div className="page narrow"><Empty title="This forum is not here" icon="lock">{err === 'not found' ? 'It does not exist, or it is private and you are not in it.' : err}</Empty></div>
  if (!doc) return <Loading />
  const f = doc.forum
  const can = doc.can
  const join = async (note = '') => {
    try {
      const d = await api.post<ForumDoc>(`${V1}/forums/${slug}/join`, { note })
      setDoc(d); setAsking(false); announceChange()
      toast(d.can.join === 'pending' ? 'Asked — a moderator will let you in' : `Welcome to ${f.name}`)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const leave = async () => {
    try { await api.post(`${V1}/forums/${slug}/leave`); announceChange(); toast(`You left ${f.name}`); load() } catch (e) { toast(errMsg(e), 'err') }
  }
  const setLevel = async (level: Level) => {
    try { await api.put(`${V1}/forums/${slug}/me`, { level }); load(); toast(level === 'watching' ? 'You hear about every new topic here' : level === 'muted' ? 'Muted: it stays out of your lists' : 'Saved') } catch (e) { toast(errMsg(e), 'err') }
  }
  const setCatLevel = async (c: Category, level: Level) => {
    try { await api.put(`${V1}/categories/${c.id}/level`, { level }); load() } catch (e) { toast(errMsg(e), 'err') }
  }
  const newTopic = () => navigate(`/new?forum=${f.slug}${cat ? `&category=${cat.slug}` : ''}`)
  const more = (): MenuItem[] => [
    ...(can.moderate ? [{ label: 'Forum settings', icon: 'settings', onSelect: () => navigate(`/f/${slug}/settings`) }, { label: 'Invite people', icon: 'users', onSelect: () => setInvite(true) }, 'sep' as const] : []),
    ...(doc.public_url ? [{ label: 'Copy the public link', icon: 'link', onSelect: () => { navigator.clipboard.writeText(doc.public_url); toast('Link copied') } }] : []),
    ...(doc.feed_url ? [{ label: 'Copy the RSS feed', icon: 'rss', onSelect: () => { navigator.clipboard.writeText(doc.feed_url); toast('Feed address copied') } }] : []),
    { label: 'Search this forum', icon: 'search', onSelect: () => navigate(`/search?forum=${slug}`) },
    ...(doc.me && doc.me.role !== 'owner' && doc.me.status === 'active' ? ['sep' as const, { label: 'Leave the forum', icon: 'log-out', danger: true, onSelect: leave }] : []),
  ]
  const tabs: { id: Tab; label: string }[] = [
    { id: 'latest', label: 'Latest' }, ...(!cat ? [{ id: 'categories' as Tab, label: 'Categories' }] : []), { id: 'top', label: 'Top' }, { id: 'unsolved', label: 'Questions' },
    { id: 'events', label: 'Events' }, ...(!cat && can.read && doc.me?.status === 'active' ? [{ id: 'members' as Tab, label: 'People' }] : []), ...(!cat ? [{ id: 'about' as Tab, label: 'About' }] : []),
  ]
  const setTab = (t: Tab) => { const n = new URLSearchParams(sp); if (t === 'latest') n.delete('tab'); else n.set('tab', t); n.delete('tag'); setSp(n, { replace: true }) }
  const subs = cat ? doc.categories.filter(c => c.parent_id === cat.id) : []
  return (
    <div className="page">
      <header className="forum-head" style={{ ['--f' as string]: forumColor(cat?.color || f.color) }}>
        {cat ? <span className="cat-big" style={{ background: `color-mix(in srgb, ${cat.color || 'var(--accent)'} 16%, transparent)` }} aria-hidden="true">{cat.emoji || '#'}</span> : <ForumMark f={f} size={52} />}
        <div className="grow">
          <h1 className="page-title">{cat ? <><Link to={`/f/${slug}`} className="crumb-forum">{f.name}</Link> <span className="muted">›</span> {cat.name}</> : f.name}</h1>
          <p className="muted">{cat ? cat.about : f.about}</p>
          <div className="chips mt-s">
            {!cat && <span className="chip-s" title={VISIBILITY[f.visibility].line}><Icon name={VISIBILITY[f.visibility].icon} size={13} />{VISIBILITY[f.visibility].label}</span>}
            {!cat && <span className="chip-s"><Icon name="users" size={13} />{doc.stats.members} {doc.stats.members === 1 ? 'member' : 'members'}</span>}
            {cat?.qa && <span className="chip-s" title="Topics here are questions that get an answer"><Icon name="check-circle" size={13} />Questions & answers</span>}
            {cat?.starts === 'mods' && <span className="chip-s"><Icon name="shield" size={13} />Moderators post here</span>}
            {f.archived && <span className="chip-s"><Icon name="archive" size={13} />Archived</span>}
            {doc.review && (doc.review.posts + doc.review.flags + doc.review.members) > 0 && <Link to="/review" className="chip-s warn"><Icon name="shield" size={13} />{doc.review.posts + doc.review.flags + doc.review.members} to review</Link>}
          </div>
        </div>
        <div className="forum-actions">
          {can.join === 'open' && <button type="button" className="btn primary" onClick={() => join()}><Icon name="plus" size={15} />Join</button>}
          {can.join === 'approval' && <button type="button" className="btn primary" onClick={() => setAsking(true)}>Ask to join</button>}
          {can.join === 'invite' && <span className="muted small">By invitation</span>}
          {can.join === 'pending' && <span className="chip-s">Waiting for a moderator</span>}
          {doc.me?.status === 'active' && (cat ? <LevelButton level={cat.level || ''} onChange={l => setCatLevel(cat, l)} what="category" /> : <LevelButton level={doc.me.level} onChange={setLevel} what="forum" />)}
          {can.post && <button type="button" className="btn primary hide-sm" onClick={newTopic}><Icon name="plus" size={15} />New topic</button>}
          <MenuButton items={more} label="More" className="btn icon-only" align="right" width={240}><Icon name="more" size={16} /></MenuButton>
        </div>
      </header>

      {subs.length > 0 && <div className="chips">{subs.map(s => <Link key={s.id} to={`/f/${slug}/c/${s.slug}`} className="chip">{s.emoji} {s.name}<span className="muted small">{s.topics}</span></Link>)}</div>}
      <div className="scroll-x"><Tabs tabs={tabs} value={tab} onChange={setTab} /></div>
      {tag && <div className="row"><span className="chip on">#{tag}<button type="button" className="chip-x" aria-label="Clear the tag" onClick={() => { const n = new URLSearchParams(sp); n.delete('tag'); setSp(n) }}><Icon name="x" size={12} /></button></span></div>}

      {tab === 'categories' ? <Categories doc={doc} />
        : tab === 'members' ? <People slug={slug} />
          : tab === 'about' ? <About doc={doc} />
            : <Topics slug={slug} forumId={f.id} cat={cat?.slug} tag={tag} filter={tab === 'latest' ? '' : tab} canPost={can.post} onNew={newTopic} />}

      {invite && <InviteModal forum={slug} owner={can.own} onClose={() => setInvite(false)} onDone={load} />}
      {asking && <AskToJoin name={f.name} rules={f.rules} onClose={() => setAsking(false)} onAsk={join} />}
    </div>
  )
}

function Topics({ slug, forumId, cat, tag, filter, canPost, onNew }: { slug: string; forumId: number; cat?: string; tag: string; filter: string; canPost: boolean; onNew: () => void }) {
  const [data, setData] = useState<TopicList | null>(null)
  const [page, setPage] = useState(0)
  const [fresh, setFresh] = useState(0)
  const [period, setPeriod] = useState('month')
  const q = useCallback((p: number) => {
    const u = new URLSearchParams({ forum: slug, limit: '30', page: String(p) })
    if (cat) u.set('category', cat)
    if (tag) u.set('tag', tag)
    if (filter) u.set('filter', filter)
    if (filter === 'top') u.set('period', period)
    return `${V1}/topics?${u}`
  }, [slug, cat, tag, filter, period])
  const load = useCallback(() => { api.get<TopicList>(q(0)).then(d => { setData(d); setPage(0); setFresh(0) }).catch(() => setData({ topics: [], forums: {}, categories: {}, more: false })) }, [q])
  useEffect(() => { setData(null); load() }, [load])
  useLive(e => { if ((e.type === 'topic' || e.type === 'post') && e.forum_id === forumId) setFresh(n => n + 1) })
  const more = async () => {
    const d = await api.get<TopicList>(q(page + 1))
    setData(cur => cur ? { ...d, topics: [...cur.topics, ...d.topics.filter(t => !cur.topics.some(x => x.id === t.id))], categories: { ...cur.categories, ...d.categories } } : d)
    setPage(p => p + 1)
  }
  return (
    <>
      {filter === 'top' && <div className="row"><SegPeriod value={period} onChange={setPeriod} /></div>}
      {fresh > 0 && <button type="button" className="new-bar" onClick={load}><Icon name="arrow-up" size={15} />{fresh === 1 ? 'Something new' : `${fresh} new posts and topics`} — show</button>}
      <div className="card flush-card">
        {!data ? <Loading /> : <TopicListView data={data} empty={
          <Empty title={filter === 'unsolved' ? 'No open questions' : filter === 'events' ? 'No events coming up' : 'No topics here yet'} icon={filter === 'events' ? 'calendar' : filter === 'unsolved' ? 'check-circle' : 'message'}>
            {canPost && <button type="button" className="btn primary" onClick={onNew}>{filter === 'events' ? 'Plan an event' : filter === 'unsolved' ? 'Ask a question' : 'Start a topic'}</button>}
          </Empty>} />}
      </div>
      {data?.more && <div className="row center"><button type="button" className="btn" onClick={more}>Show more</button></div>}
    </>
  )
}

function SegPeriod({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="segmented sm" role="radiogroup" aria-label="Period">
      {[['week', 'Week'], ['month', 'Month'], ['year', 'Year'], ['all', 'All time']].map(([v, l]) => <button key={v} type="button" role="radio" aria-checked={value === v} className={value === v ? 'active' : ''} onClick={() => onChange(v)}>{l}</button>)}
    </div>
  )
}

function Categories({ doc }: { doc: ForumDoc }) {
  const top = doc.categories.filter(c => !c.parent_id)
  return (
    <div className="cat-grid">
      {top.map(c => {
        const subs = doc.categories.filter(s => s.parent_id === c.id)
        return (
          <Link key={c.id} to={`/f/${doc.forum.slug}/c/${c.slug}`} className={`cat-card ${c.archived ? 'dim' : ''}`} style={{ ['--c' as string]: c.color || 'var(--accent)' }}>
            <div className="cat-card-head"><span className="cat-ico" aria-hidden="true">{c.emoji || '#'}</span><b className="grow">{c.name}</b>{c.qa && <Icon name="check-circle" size={14} className="muted" title="Questions & answers" />}{c.starts === 'mods' && <Icon name="shield" size={14} className="muted" title="Moderators post here" />}</div>
            {c.about && <p className="muted small">{c.about}</p>}
            {subs.length > 0 && <div className="chips">{subs.map(s => <span key={s.id} className="chip-s">{s.emoji} {s.name}</span>)}</div>}
            <div className="cat-card-foot muted small"><span>{c.topics} {c.topics === 1 ? 'topic' : 'topics'}</span><span>{c.posts} posts</span>{c.latest_at && <span>latest <When at={c.latest_at} /></span>}</div>
          </Link>
        )
      })}
    </div>
  )
}

function People({ slug }: { slug: string }) {
  const [list, setList] = useState<Member[] | null>(null)
  const [q, setQ] = useState('')
  useEffect(() => {
    const t = window.setTimeout(() => api.get<{ members: Member[] }>(`${V1}/forums/${slug}/members?status=active&q=${encodeURIComponent(q)}`).then(r => setList(r.members)).catch(() => setList([])), 200)
    return () => window.clearTimeout(t)
  }, [slug, q])
  return (
    <div className="stack">
      <div className="add-search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find someone" /></div>
      <div className="card flush-card">
        {!list ? <Loading /> : list.length === 0 ? <Empty title="Nobody matches" icon="users" /> : (
          <div className="rows">
            {list.map(m => (
              <Link key={m.user_id} to={`/u/${m.user_id}`} className="row person-row">
                <Face name={m.name} color={m.color} avatarId={m.avatar_id} size={36} />
                <span className="grow"><b>{m.name} <span className="muted small">@{m.handle}</span>{m.role !== 'member' && <span className={`role-pill ${m.role}`}>{m.role === 'owner' ? 'Owner' : 'Moderator'}</span>}{m.title && <span className="role-pill">{m.title}</span>}</b>
                  <span className="muted small">{m.posts_n} posts · {m.likes_got} reactions · {m.solutions_n} solutions · joined {fmtDate(m.joined_at.slice(0, 10))}</span></span>
                <span className="badges-mini">{m.badges?.slice(0, 4).map(b => <span key={b.id} title={`${b.name}: ${b.line}`}>{b.emoji}</span>)}</span>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function About({ doc }: { doc: ForumDoc }) {
  const f = doc.forum
  return (
    <div className="about-grid">
      <div className="stack">
        {f.rules ? <section className="card pad-card"><h3 className="card-h">How we do things here</h3><Markdown text={f.rules} /></section> : <section className="card pad-card muted">This forum has no rules written down yet.</section>}
        {f.welcome && <section className="card pad-card"><h3 className="card-h">Welcome</h3><Markdown text={f.welcome} /></section>}
      </div>
      <div className="stack">
        <div className="stat-row">
          <Stat label="Topics" value={doc.stats.topics} sub={`${doc.stats.week_topics} this week`} />
          <Stat label="Posts" value={doc.stats.posts} sub={`${doc.stats.week_posts} this week`} />
          <Stat label="Members" value={doc.stats.members} />
          <Stat label="Solved" value={doc.stats.solved} />
        </div>
        <section className="card pad-card">
          <h3 className="card-h">Run by</h3>
          <div className="stack tight">{doc.staff.map(s => <Link key={s.user_id} to={`/u/${s.user_id}`} className="row person-mini"><Face name={s.name} color={s.color} avatarId={s.avatar_id} size={28} /><span className="grow">{s.name} <span className="muted small">@{s.handle}</span></span><span className={`role-pill ${s.role}`}>{s.role === 'owner' ? 'Owner' : 'Moderator'}</span></Link>)}</div>
        </section>
        <section className="card pad-card stack">
          <div className="muted small"><Icon name={VISIBILITY[f.visibility].icon} size={13} /> {VISIBILITY[f.visibility].line}. {JOIN[f.join_policy].label}. Started {fmtDate(f.created_at.slice(0, 10))}.</div>
          {doc.public_url && <CopyField value={doc.public_url} label="Public link" />}
          {doc.feed_url && <CopyField value={doc.feed_url} label="RSS feed" />}
        </section>
      </div>
    </div>
  )
}

function AskToJoin({ name, rules, onClose, onAsk }: { name: string; rules: string; onClose: () => void; onAsk: (note: string) => Promise<void> }) {
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  return (
    <Modal title={`Ask to join ${name}`} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={busy} onClick={async () => { setBusy(true); await onAsk(note); setBusy(false) }}>{busy && <Spinner size={14} />}Ask</button></>}>
      <div className="stack">
        <p className="muted">A moderator lets people in. A line about who you are helps.</p>
        <textarea value={note} onChange={e => setNote(e.target.value)} rows={3} maxLength={500} placeholder="Hi, I'm Lejla's neighbour from no. 12…" autoFocus />
        {rules && <details><summary className="muted small">The forum's rules</summary><Markdown text={rules} /></details>}
      </div>
    </Modal>
  )
}
