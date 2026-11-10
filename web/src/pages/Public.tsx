// Pages for people without an account: a public forum and its topics (with
// guest posts when the forum allows them), and an invitation to join.
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { eventWhen } from '../lib/forum'
import type { ForumDoc, ForumMini, TopicDoc, TopicList } from '../lib/types'
import { Icon } from '../components/icons'
import { CatChip, ForumMark, TagChips, TopicMarks, TopicRow } from '../components/forum'
import { Markdown } from '../components/Markdown'
import { PostView } from '../components/PostView'
import { Empty, Field, Loading, Spinner, Tabs, errMsg, useToast } from '../components/ui'

function PubShell({ forum, children }: { forum?: { slug: string; name: string; emoji: string; color: string }; children: React.ReactNode }) {
  const { user } = useApp()
  return (
    <div className="pub">
      <header className="pub-top">
        {forum ? <Link to={`/p/${forum.slug}`} className="pub-brand"><ForumMark f={forum} size={30} /><b>{forum.name}</b></Link> : <span className="pub-brand"><img src="/favicon.svg" alt="" width={28} height={28} /><b>Pocket Forum</b></span>}
        <span className="grow" />
        {user ? <Link to={forum ? `/f/${forum.slug}` : '/'} className="btn">Open in Pocket Forum</Link> : <Link to="/login" className="btn">Sign in</Link>}
      </header>
      <main className="pub-main">{children}</main>
      <footer className="pub-foot muted small">Pocket Forum · <a href="https://github.com/abjelosevic88/pocket-homelab-forum" target="_blank" rel="noreferrer">Run your own</a></footer>
    </div>
  )
}

export function PublicForum() {
  const { slug = '', cat } = useParams()
  const [doc, setDoc] = useState<ForumDoc | null>(null)
  const [list, setList] = useState<TopicList | null>(null)
  const [err, setErr] = useState('')
  const [tab, setTab] = useState<'latest' | 'top' | 'unsolved' | 'about'>('latest')
  const [asking, setAsking] = useState(false)
  useEffect(() => { api.get<ForumDoc>(`${V1}/public/forums/${slug}`).then(setDoc).catch(e => setErr(errMsg(e))) }, [slug])
  useEffect(() => {
    if (tab === 'about') return
    setList(null)
    const q = new URLSearchParams({ filter: tab === 'latest' ? '' : tab })
    if (cat) q.set('category', cat)
    api.get<TopicList>(`${V1}/public/forums/${slug}/topics?${q}`).then(setList).catch(() => setList({ topics: [], forums: {}, categories: {}, more: false }))
  }, [slug, cat, tab])
  if (err) return <PubShell><Empty title="This forum is not public" icon="lock">It does not exist, or only its members can read it.</Empty></PubShell>
  if (!doc) return <PubShell><Loading /></PubShell>
  const f = doc.forum
  const category = doc.categories.find(c => c.slug === cat)
  return (
    <PubShell forum={f}>
      <div className="page">
        <header className="forum-head"><ForumMark f={f} size={52} /><div className="grow"><h1 className="page-title">{category ? `${f.name} › ${category.name}` : f.name}</h1><p className="muted">{category?.about || f.about}</p></div>
          {doc.guest_posts && <button type="button" className="btn primary" onClick={() => setAsking(true)}><Icon name="plus" size={15} />Ask a question</button>}</header>
        <div className="chips">{!cat ? doc.categories.filter(c => !c.parent_id && !c.archived).map(c => <Link key={c.id} to={`/p/${slug}/c/${c.slug}`} className="chip">{c.emoji} {c.name}<span className="muted small">{c.topics}</span></Link>) : <Link to={`/p/${slug}`} className="chip"><Icon name="arrow-left" size={13} />All categories</Link>}</div>
        <Tabs tabs={[{ id: 'latest', label: 'Latest' }, { id: 'top', label: 'Top' }, { id: 'unsolved', label: 'Open questions' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
        {asking && <GuestForm slug={slug} categories={doc.categories.filter(c => c.starts === 'all' && !c.archived)} onDone={() => setAsking(false)} />}
        {tab === 'about' ? <div className="card pad-card">{f.rules ? <Markdown text={f.rules} pub /> : <p className="muted">No rules written down yet.</p>}</div> : (
          <div className="card flush-card">{!list ? <Loading /> : list.topics.length === 0 ? <Empty title="No topics yet" icon="message" /> : (
            <div className="t-list" role="list">{list.topics.map(t => <PubRow key={t.id} slug={slug} t={t} cat={list.categories[t.category_id]} forum={list.forums[t.forum_id]} />)}</div>
          )}</div>
        )}
        {doc.feed_url && <p className="muted small"><Icon name="rss" size={12} /> <a href={doc.feed_url}>RSS feed</a></p>}
      </div>
    </PubShell>
  )
}

function PubRow({ slug, t, cat, forum }: { slug: string; t: TopicList['topics'][number]; cat?: TopicList['categories'][string]; forum?: ForumMini }) {
  const navigate = useNavigate()
  return <div onClickCapture={e => { if (!(e.target as HTMLElement).closest('.cat-chip, .tag-chip')) { e.preventDefault(); e.stopPropagation(); navigate(`/p/${slug}/t/${t.id}`) } }}><TopicRow t={t} cat={cat} forum={forum} /></div>
}

function GuestForm({ slug, topic, categories, onDone }: { slug: string; topic?: number; categories?: ForumDoc['categories']; onDone: () => void }) {
  const { toast } = useToast()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [cat, setCat] = useState(String(categories?.find(c => c.qa)?.id || categories?.[0]?.id || ''))
  const [website, setWebsite] = useState('')
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(false)
  const send = async () => {
    setBusy(true)
    try {
      if (topic) await api.post(`${V1}/public/topics/${topic}/posts`, { body, guest_name: name, guest_email: email, website })
      else await api.post(`${V1}/public/forums/${slug}/topics`, { title, body, category_id: Number(cat) || 0, guest_name: name, guest_email: email, website })
      setSent(true)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  if (sent) return <div className="card pad-card"><b><Icon name="check-circle" size={16} className="ok" /> Thank you!</b><p className="muted">A moderator reads it first; it shows here once approved.</p><button type="button" className="btn" onClick={onDone}>Close</button></div>
  return (
    <div className="card pad-card stack guest-form">
      <b>{topic ? 'Reply as a guest' : 'Ask as a guest'}</b>
      <div className="grid two"><Field label="Your name"><input value={name} onChange={e => setName(e.target.value)} maxLength={60} /></Field><Field label="Your email (not shown)"><input value={email} onChange={e => setEmail(e.target.value)} type="email" /></Field></div>
      {!topic && <><Field label="Title"><input value={title} onChange={e => setTitle(e.target.value)} maxLength={200} /></Field>
        {categories && categories.length > 0 && <Field label="Category" group><div className="chips">{categories.map(c => <button key={c.id} type="button" className={`chip ${String(c.id) === cat ? 'on' : ''}`} onClick={() => setCat(String(c.id))}>{c.emoji} {c.name}</button>)}</div></Field>}</>}
      <Field label={topic ? 'Your reply' : 'Your question'}><textarea value={body} onChange={e => setBody(e.target.value)} rows={5} /></Field>
      <input className="hp" value={website} onChange={e => setWebsite(e.target.value)} tabIndex={-1} autoComplete="off" aria-hidden="true" placeholder="Leave this empty" />
      <div className="row"><span className="grow muted small">Guest posts wait for a moderator.</span><button type="button" className="btn" onClick={onDone}>Cancel</button>
        <button type="button" className="btn primary" disabled={busy || !name.trim() || body.trim().length < 2 || (!topic && title.trim().length < 3)} onClick={send}>{busy && <Spinner size={14} />}Send</button></div>
    </div>
  )
}

export function PublicTopic() {
  const { slug = '', id = '' } = useParams()
  const [doc, setDoc] = useState<TopicDoc | null>(null)
  const [err, setErr] = useState('')
  const [reply, setReply] = useState(false)
  useEffect(() => { api.get<TopicDoc>(`${V1}/public/topics/${id}`).then(setDoc).catch(e => setErr(errMsg(e))) }, [id])
  useEffect(() => { if (doc && location.hash) setTimeout(() => document.getElementById(location.hash.slice(1))?.scrollIntoView(), 100) }, [doc])
  if (err) return <PubShell><Empty title="This topic is not here" icon="message" /></PubShell>
  if (!doc) return <PubShell><Loading /></PubShell>
  const t = doc.topic
  const noop = () => {}
  const jump = (n: number) => document.getElementById(`p${n}`)?.scrollIntoView({ behavior: 'smooth' })
  const pubDoc = { ...doc, can: { ...doc.can, reply: false, member: false, moderate: false, solve: false, edit_topic: false } }
  return (
    <PubShell forum={{ ...doc.forum, slug }}>
      <div className="page topic-page">
        <header className="topic-head">
          <div className="topic-forum">{doc.category && <CatChip c={doc.category} />}<TagChips tags={t.tags} /></div>
          <h1 className="topic-title"><TopicMarks t={t} /><span>{t.title}</span></h1>
          {t.kind === 'event' && t.event_start && <p className="muted"><Icon name="calendar" size={14} /> {eventWhen(t.event_start, t.event_end)}{t.event_place ? ` · ${t.event_place}` : ''}</p>}
        </header>
        <div className="posts">{doc.posts.filter(p => p.state === 'visible').map(p => <PostView key={p.id} p={p} doc={pubDoc} pub act={{ reply: noop, quote: noop, edit: noop, changed: noop, jump }} />)}</div>
        {doc.guest_posts ? (reply ? <GuestForm slug={slug} topic={t.id} onDone={() => setReply(false)} /> : <button type="button" className="reply-start" onClick={() => setReply(true)}><Icon name="reply" size={16} />Reply as a guest</button>)
          : <p className="muted center small"><Link to="/login">Sign in</Link> to reply.</p>}
      </div>
    </PubShell>
  )
}

interface InviteInfo { forum: ForumMini; about: string; members: number; topics: number; by: string; note: string; email: string; role: string; why: string; accounts: boolean; oidc: boolean; oidc_name: string }

export function Join() {
  const { token = '' } = useParams()
  const navigate = useNavigate()
  const { user, refresh } = useApp()
  const { toast } = useToast()
  const [info, setInfo] = useState<InviteInfo | null>(null)
  const [err, setErr] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<InviteInfo>(`${V1}/public/invites/${token}`).then(i => { setInfo(i); if (i.email) setEmail(i.email) }).catch(e => setErr(errMsg(e))) }, [token])
  const accept = async () => {
    setBusy(true)
    try { const r = await api.post<{ slug: string }>(`${V1}/invites/${token}/accept`); navigate(`/f/${r.slug}`) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const signup = async () => {
    setBusy(true)
    try { const r = await api.post<{ slug: string }>(`${V1}/public/invites/${token}/signup`, { name, email, password }); await refresh(); navigate(`/f/${r.slug}`) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  if (err) return <PubShell><Empty title="This invitation does not work" icon="mail">{err}</Empty></PubShell>
  if (!info) return <PubShell><Loading /></PubShell>
  return (
    <PubShell>
      <div className="join-card card pad-card">
        <ForumMark f={info.forum} size={64} />
        <h1>{info.by ? `${info.by} invites you to` : 'You are invited to'} {info.forum.name}</h1>
        {info.about && <p className="muted">{info.about}</p>}
        <p className="muted small">{info.members} members · {info.topics} topics{info.role === 'moderator' ? ' · you would be a moderator' : ''}</p>
        {info.why ? <p className="error-text">{info.why[0].toUpperCase() + info.why.slice(1)}.</p> : user ? (
          <button type="button" className="btn primary lg" disabled={busy} onClick={accept}>{busy && <Spinner size={14} />}Join as {user.name || user.email}</button>
        ) : (
          <div className="stack join-form">
            {info.accounts && <>
              <Field label="Your name"><input value={name} onChange={e => setName(e.target.value)} autoFocus /></Field>
              <Field label="Email"><input value={email} onChange={e => setEmail(e.target.value)} type="email" disabled={!!info.email} /></Field>
              <Field label="A password" help="At least 8 characters."><input value={password} onChange={e => setPassword(e.target.value)} type="password" autoComplete="new-password" /></Field>
              <button type="button" className="btn primary lg" disabled={busy || !name.trim() || !email.includes('@') || password.length < 8} onClick={signup}>{busy && <Spinner size={14} />}Make my account and join</button>
              <p className="muted small center">Already have an account here?</p>
            </>}
            <Link to="/login" state={{ from: `/join/${token}` }} className="btn lg">Sign in{info.oidc ? '' : ' with your account'}</Link>
          </div>
        )}
      </div>
    </PubShell>
  )
}
