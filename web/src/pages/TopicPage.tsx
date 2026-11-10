// A topic: its posts in order, where you left off, the reply box, and the
// topic's tools — follow, share, summary, event answers, and for moderators
// closing, pinning, moving, merging and splitting. New replies appear at the
// bottom as they come (or as a bar, when you are reading further up).
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, openAction, useAppState } from '../lib/data'
import { useLive } from '../lib/live'
import { KINDS, eventParts, eventWhen } from '../lib/forum'
import type { Category, ForumDoc, Hit, Level, Post, RSVP, Topic, TopicDoc } from '../lib/types'
import { Icon } from '../components/icons'
import { AskText, CatChip, LevelButton, TagChips, TopicMarks } from '../components/forum'
import { Composer, type ComposerHandle } from '../components/Composer'
import { PostView } from '../components/PostView'
import { Markdown, plain } from '../components/Markdown'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Field, Loading, type MenuItem, MenuButton, Modal, Segmented, Spinner, errMsg, useDebounce, useIsMobile, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { TagInput } from './NewTopic'

export default function TopicPage() {
  const { id = '', n } = useParams()
  const tid = Number(id)
  const navigate = useNavigate()
  const { toast } = useToast()
  const { user } = useApp()
  const mobile = useIsMobile()
  const appState = useAppState()
  const [doc, setDoc] = useState<TopicDoc | null>(null)
  const [err, setErr] = useState('')
  const [reply, setReply] = useState('')
  const [replyTo, setReplyTo] = useState<Post | null>(null)
  const [editing, setEditing] = useState<Post | null>(null)
  const [editText, setEditText] = useState('')
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(0)
  const [focus, setFocus] = useState(0)
  const [summary, setSummary] = useState<string | null>(null)
  const [sumBusy, setSumBusy] = useState(false)
  const [modal, setModal] = useState<'' | 'move' | 'merge' | 'pin' | 'timer' | 'edit-title' | 'delete' | 'email'>('')
  const [splitting, setSplitting] = useState<number[] | null>(null)
  const composer = useRef<ComposerHandle | null>(null)
  const replyBox = useRef<HTMLDivElement>(null)
  const seen = useRef(0)
  const scrolled = useRef(false)

  const load = useCallback(async (quiet = false) => {
    try {
      const d = await api.get<TopicDoc>(`${V1}/topics/${tid}`)
      if (d.moved_to) { navigate(`/t/${d.moved_to}`, { replace: true }); return }
      setDoc(d)
      setErr('')
      if (!quiet) setPending(0)
    } catch (e) { setErr(errMsg(e)) }
  }, [tid, navigate])
  useEffect(() => { setDoc(null); scrolled.current = false; seen.current = 0; load() }, [load])
  useCrumb(doc?.topic.title, doc ? `/f/${doc.forum.slug}` : undefined)

  // a draft for this reply, from before
  const draftKey = `reply:${tid}`
  useEffect(() => {
    api.get<{ data: { body?: string; reply_to?: number } | null }>(`${V1}/drafts/${draftKey}`).then(r => { if (r.data?.body) { setReply(r.data.body); setOpen(true) } }).catch(() => {})
  }, [draftKey])

  // where to start: the post in the address, else where you left off
  useEffect(() => {
    if (!doc || scrolled.current) return
    scrolled.current = true
    const target = n ? Number(n) : doc.topic.visited && doc.topic.unread > 0 ? doc.topic.last_read + 1 : 0
    if (target > 1) requestAnimationFrame(() => jump(target, false))
  }, [doc, n]) // eslint-disable-line react-hooks/exhaustive-deps

  // reading: the highest post that was on screen counts as read
  const [readTick, setReadTick] = useState(0)
  useEffect(() => {
    if (!doc) return
    const io = new IntersectionObserver(entries => {
      for (const en of entries) {
        if (!en.isIntersecting) continue
        const num = Number((en.target as HTMLElement).dataset.n)
        if (num > seen.current) { seen.current = num; setReadTick(t => t + 1) }
        setFocus(num)
      }
    }, { rootMargin: '0px 0px -30% 0px', threshold: 0.2 })
    document.querySelectorAll('article.post').forEach(el => io.observe(el))
    return () => io.disconnect()
  }, [doc])
  useEffect(() => {
    if (!readTick || !doc?.can.member) return
    const t = window.setTimeout(() => {
      if (seen.current > (doc.topic.last_read || 0)) api.post(`${V1}/topics/${tid}/read`, { number: seen.current }).then(() => announceChange()).catch(() => {})
    }, 1200)
    return () => window.clearTimeout(t)
  }, [readTick]) // eslint-disable-line react-hooks/exhaustive-deps

  // live: replies at the bottom when you are there; else a bar
  const nearBottom = () => window.innerHeight + (document.querySelector('.content')?.scrollTop || 0) >= (document.querySelector('.content')?.scrollHeight || 0) - 400
  useLive(e => {
    if (e.topic_id !== tid) return
    if (e.type === 'post') {
      if (e.user_id === user?.id) return
      if (nearBottom()) load(true)
      else setPending(p => p + 1)
    } else if (e.type === 'edit' || e.type === 'topic' || e.type === 'poll' || e.type === 'rsvp') load(true)
  })

  const jump = (num: number, smooth = true) => {
    const el = document.getElementById(`p${num}`)
    if (!el) { if (doc && num > 0) navigate(`/t/${tid}/${num}`, { replace: true }); return }
    el.scrollIntoView({ behavior: smooth ? 'smooth' : 'auto', block: 'start' })
    setFocus(num)
    el.classList.add('flash')
    window.setTimeout(() => el.classList.remove('flash'), 1400)
  }
  const openReply = (to: Post | null) => {
    setReplyTo(to)
    setOpen(true)
    requestAnimationFrame(() => { replyBox.current?.scrollIntoView({ behavior: 'smooth', block: 'end' }); composer.current?.focus() })
  }
  const quote = (p: Post) => {
    const sel = window.getSelection()?.toString().trim()
    const text = sel && document.getElementById(`p${p.number}`)?.contains(window.getSelection()?.anchorNode || null) ? sel : plain(p.body, 600)
    const q = `[quote=${p.handle || p.author_name.replace(/\W+/g, '')} #${p.number}]\n${text}\n[/quote]\n\n`
    setReplyTo(p)
    setOpen(true)
    requestAnimationFrame(() => { composer.current?.insert(q); replyBox.current?.scrollIntoView({ behavior: 'smooth', block: 'end' }) })
  }
  const send = async () => {
    if (!reply.trim()) return
    setBusy(true)
    try {
      const r = await api.post<{ post: Post; state: string }>(`${V1}/topics/${tid}/posts`, { body: reply, reply_to: replyTo?.number || 0, draft_key: draftKey })
      setReply(''); setReplyTo(null); setOpen(false)
      toast(r.state === 'pending' ? 'Sent — a moderator will look at it before it shows' : 'Posted')
      await load(true)
      requestAnimationFrame(() => jump(r.post.number))
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const saveEdit = async () => {
    if (!editing) return
    setBusy(true)
    try { await api.put(`${V1}/posts/${editing.id}`, { body: editText }); setEditing(null); toast('Saved'); load(true) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const status = async (body: object, ok?: string) => {
    try { await api.post(`${V1}/topics/${tid}/status`, body); if (ok) toast(ok); load(true); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const setLevel = async (level: Level) => {
    try { await api.put(`${V1}/topics/${tid}/level`, { level }); load(true) } catch (e) { toast(errMsg(e), 'err') }
  }
  const summarize = async (since = 0) => {
    setSumBusy(true)
    try { setSummary((await api.post<{ summary: string }>(`${V1}/ai/summary`, { topic_id: tid, since })).summary) } catch (e) { toast(errMsg(e), 'err') } finally { setSumBusy(false) }
  }
  const share = async (path: string, body: object = {}) => {
    try { const r = await api.post<{ text: string; url?: string }>(`${V1}${path}`, { topic_id: tid, ...body }); toast(r.text, 'ok', openAction(r.url)) } catch (e) { toast(errMsg(e), 'err') }
  }

  // keys: j/k between posts, r to reply, l to react, s for the summary
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (e.metaKey || e.ctrlKey || e.altKey || t.closest('input, textarea, [contenteditable="true"]') || document.querySelector('.modal-backdrop, .menu, .popover')) return
      if (!doc) return
      const nums = doc.posts.map(p => p.number)
      const i = Math.max(0, nums.indexOf(focus))
      if (e.key === 'j') { e.preventDefault(); jump(nums[Math.min(nums.length - 1, i + 1)]) }
      else if (e.key === 'k') { e.preventDefault(); jump(nums[Math.max(0, i - 1)]) }
      else if ((e.key === 'r' || e.key === 'R') && doc.can.reply) { e.preventDefault(); openReply(e.key === 'R' ? null : doc.posts.find(p => p.number === focus) || null) }
      else if (e.key === 's') { e.preventDefault(); summarize() }
      else if (e.key === 'l') { e.preventDefault(); (document.querySelector(`#p${focus} .react.add`) as HTMLButtonElement | null)?.click() }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  if (err) return <div className="page narrow"><Empty title="This topic is not here" icon="message">{err === 'not found' ? 'It was removed, or it is in a forum you are not in.' : err}</Empty></div>
  if (!doc) return <Loading />
  const t = doc.topic
  const can = doc.can
  const solution = t.solved_post_id ? doc.posts.find(p => p.id === t.solved_post_id) : undefined
  const modItems = (): MenuItem[] => [
    { heading: 'Moderate' },
    { label: t.closed ? 'Open it again' : 'Close (no more replies)', icon: t.closed ? 'unlock' : 'lock', onSelect: () => status({ closed: !t.closed }, t.closed ? 'Open again' : 'Closed') },
    { label: t.pinned ? 'Unpin' : 'Pin…', icon: 'pin', onSelect: () => t.pinned ? status({ pinned: 0 }, 'Unpinned') : setModal('pin') },
    { label: 'Move to another category…', icon: 'move', onSelect: () => setModal('move') },
    { label: 'Merge into another topic…', icon: 'merge', onSelect: () => setModal('merge') },
    { label: 'Move some posts out…', icon: 'split', onSelect: () => setSplitting([]) },
    { label: 'Close later, slow mode…', icon: 'timer', onSelect: () => setModal('timer') },
    { label: t.unlisted ? 'List it again' : 'Unlist (only its link finds it)', icon: t.unlisted ? 'eye' : 'eye-off', onSelect: () => status({ unlisted: !t.unlisted }) },
    { label: t.archived ? 'Unarchive' : 'Archive (read only for good)', icon: 'archive', onSelect: () => status({ archived: !t.archived }) },
    'sep',
    { label: 'Remove the topic', icon: 'trash', danger: true, onSelect: () => setModal('delete') },
  ]
  const shareItems = (): MenuItem[] => {
    const chat = appState('chat', 'chat:write'), email = appState('email', 'mail:send'), notes = appState('notes', 'notes:write'), files = appState('files', 'files:write'), blog = appState('blog', 'blog:write'), cal = appState('calendar', 'calendar:write')
    return [
      { label: 'Copy the link', icon: 'link', onSelect: () => { navigator.clipboard.writeText(`${location.origin}/t/${t.id}`); toast('Link copied') } },
      ...(doc.public_url ? [{ label: 'Copy the public link', icon: 'globe', onSelect: () => { navigator.clipboard.writeText(doc.public_url!); toast('Public link copied') } }] : []),
      'sep',
      { label: 'Share in Pocket Chat', icon: 'message', disabled: !chat.ok, hint: chat.ok ? undefined : 'not connected', onSelect: () => share('/apps/chat') },
      { label: 'Email it…', icon: 'mail', disabled: !email.ok, hint: email.ok ? undefined : 'not connected', onSelect: () => setModal('email') },
      ...(t.kind === 'event' ? [{ label: 'Add to Pocket Calendar', icon: 'calendar-plus', disabled: !cal.ok, hint: cal.ok ? undefined : 'not connected', onSelect: () => share('/apps/calendar') }] : []),
      'sep',
      { label: 'Save the topic in Pocket Notes', icon: 'note', disabled: !notes.ok, hint: notes.ok ? undefined : 'not connected', onSelect: () => share('/apps/notes') },
      { label: 'Save as a file in Pocket Files', icon: 'file', disabled: !files.ok, hint: files.ok ? undefined : 'not connected', onSelect: () => share('/apps/files') },
      { label: 'Start a blog post from it', icon: 'feather', disabled: !blog.ok, hint: blog.ok ? undefined : 'not connected', onSelect: () => share('/apps/blog') },
    ]
  }
  const posts = doc.posts
  const firstUnread = t.visited && t.unread > 0 ? t.last_read + 1 : 0
  return (
    <div className="page topic-page">
      <header className="topic-head">
        <div className="topic-forum"><Link to={`/f/${doc.forum.slug}`} className="t-forum"><span aria-hidden="true">{doc.forum.emoji}</span>{doc.forum.name}</Link>{doc.category && <CatChip c={doc.category} to={`/f/${doc.forum.slug}/c/${doc.category.slug}`} />}<TagChips tags={t.tags} forum={doc.forum.slug} /></div>
        <h1 className="topic-title"><TopicMarks t={t} /><span>{t.title}</span>
          {can.edit_topic && <button type="button" className="btn ghost icon-only sm" aria-label="Change the title, category and tags" title="Change the title, category and tags" onClick={() => setModal('edit-title')}><Icon name="edit" size={15} /></button>}</h1>
        <div className="topic-tools">
          <span className="muted small">{t.posts_n} {t.posts_n === 1 ? 'post' : 'posts'} · {t.views} views{t.likes_n ? ` · ${t.likes_n} reactions` : ''}</span>
          <span className="grow" />
          {can.member && <LevelButton level={t.level} onChange={setLevel} what="topic" compact={mobile} />}
          <button type="button" className="btn" onClick={() => summarize()} disabled={sumBusy} title="Summary by AI (S)">{sumBusy ? <Spinner size={14} /> : <Icon name="sparkles" size={15} />}<span className="hide-sm">Summary</span></button>
          <MenuButton items={shareItems} label="Share" className="btn" align="right" width={270}><Icon name="share" size={15} /><span className="hide-sm">Share</span></MenuButton>
          {can.moderate && <MenuButton items={modItems} label="Moderate" className="btn icon-only" align="right" width={270}><Icon name="shield" size={16} /></MenuButton>}
        </div>
      </header>

      {summary !== null && (
        <section className="card pad-card ai-card">
          <div className="row"><b className="grow"><Icon name="sparkles" size={15} /> Summary</b>
            {appState('notes', 'notes:write').ok && <button type="button" className="btn sm" onClick={() => share('/apps/notes', { summary })}><Icon name="note" size={14} />Save to Notes</button>}
            <button type="button" className="btn ghost icon-only sm" aria-label="Close" onClick={() => setSummary(null)}><Icon name="x" size={15} /></button></div>
          <Markdown text={summary} topicId={t.id} onQuoteClick={jump} />
          <p className="muted small">Written by AI from the posts — check what matters in the posts themselves.</p>
        </section>
      )}
      {t.kind === 'event' && <EventCard t={t} rsvps={doc.rsvps || []} canAnswer={can.member} me={user?.id || 0} onChanged={() => load(true)} onCalendar={appState('calendar', 'calendar:write').ok ? () => share('/apps/calendar') : undefined} />}
      {solution && <button type="button" className="solved-banner" onClick={() => jump(solution.number)}><Icon name="check-circle" size={16} /><span className="grow">Solved by <b>{solution.author_name}</b> in #{solution.number}: {plain(solution.body, 140)}</span><Icon name="arrow-down" size={15} /></button>}
      {firstUnread > 1 && !n && <button type="button" className="unread-jump" onClick={() => jump(firstUnread)}>You read up to #{t.last_read} — jump to the {t.unread} new {t.unread === 1 ? 'post' : 'posts'} <Icon name="arrow-down" size={14} />{t.unread > 6 && <span className="linkish" onClick={e => { e.stopPropagation(); summarize(t.last_read) }}> · or catch me up</span>}</button>}
      {t.closed && <div className="post-note"><Icon name="lock" size={14} />Closed{t.archived ? ' and archived' : ''}: no more replies.{can.moderate && ' Moderators can still post.'}</div>}

      {splitting && <div className="split-bar card pad-card row wrap"><b className="grow">Pick the posts to move ({splitting.length})</b><button type="button" className="btn" onClick={() => setSplitting(null)}>Cancel</button><SplitButton tid={tid} ids={splitting} doc={doc} onDone={id => { setSplitting(null); navigate(`/t/${id}`) }} /></div>}

      <div className="posts">
        {posts.map((p, i) => (
          <React.Fragment key={p.id}>
            {firstUnread > 1 && p.number === firstUnread && i > 0 && <div className="unread-line"><span>new since you last read</span></div>}
            {editing?.id === p.id ? (
              <div className="post editing" id={`p${p.number}`} data-n={p.number}>
                <Composer value={editText} onChange={setEditText} forum={doc.forum.slug} onSubmit={saveEdit} submitLabel="Save" busy={busy} onCancel={() => setEditing(null)} autoFocus topicId={t.id} />
              </div>
            ) : (
              <PostView p={p} doc={doc} focused={focus === p.number} selectable={!!splitting} selected={splitting?.includes(p.id)} onSelect={on => setSplitting(s => s ? (on ? [...s, p.id] : s.filter(x => x !== p.id)) : s)}
                act={{ reply: openReply, quote, edit: x => { setEditing(x); setEditText(x.body) }, changed: () => load(true), jump }} />
            )}
          </React.Fragment>
        ))}
      </div>

      {pending > 0 && <button type="button" className="new-bar bottom" onClick={async () => { await load(); requestAnimationFrame(() => document.querySelector('.content')?.scrollTo({ top: 1e9, behavior: 'smooth' })) }}><Icon name="arrow-down" size={15} />{pending === 1 ? '1 new reply' : `${pending} new replies`}</button>}

      <div ref={replyBox} className="reply-zone">
        {can.reply ? (open ? (
          <div className="card reply-card">
            {replyTo && <div className="replying-to">Replying to <b>{replyTo.author_name}</b> <button type="button" className="linkish" onClick={() => jump(replyTo.number)}>#{replyTo.number}</button><button type="button" className="chip-x" aria-label="Reply to the topic instead" onClick={() => setReplyTo(null)}><Icon name="x" size={12} /></button></div>}
            <Composer value={reply} onChange={setReply} forum={doc.forum.slug} draftKey={draftKey} draftExtra={{ reply_to: replyTo?.number || 0 }} onSubmit={send} submitLabel="Reply" busy={busy}
              placeholder={t.kind === 'question' ? 'Know the answer? Explain it simply.' : 'Write your reply…'} topicId={t.id} replyTo={replyTo?.number} autoFocus handle={composer}
              onCancel={() => { setOpen(false); setReplyTo(null) }} />
          </div>
        ) : (
          <button type="button" className="reply-start" onClick={() => openReply(null)}><Icon name="reply" size={16} />{reply ? 'Go on with your reply' : 'Write a reply…'}</button>
        )) : can.join ? <JoinHint forum={doc.forum.slug} join={can.join} onJoined={() => load()} /> : can.why ? <p className="muted center small">{can.why[0].toUpperCase() + can.why.slice(1)}.</p> : null}
      </div>

      {!mobile && posts.length > 6 && <div className="progress-pill" aria-hidden="true"><button type="button" className="btn ghost icon-only sm" onClick={() => jump(posts[0].number)} title="Back to the top"><Icon name="chevron-up" size={15} /></button><span>{focus || 1} / {t.highest}</span><button type="button" className="btn ghost icon-only sm" onClick={() => jump(posts[posts.length - 1].number)} title="To the last post"><Icon name="chevron-down" size={15} /></button></div>}

      {modal === 'edit-title' && <EditTopic t={t} slug={doc.forum.slug} mod={can.moderate} onClose={() => setModal('')} onSaved={() => { setModal(''); load(true) }} />}
      {modal === 'move' && <MoveTopic t={t} slug={doc.forum.slug} onClose={() => setModal('')} onSaved={() => { setModal(''); load(true) }} />}
      {modal === 'merge' && <MergeTopic t={t} slug={doc.forum.slug} onClose={() => setModal('')} onDone={id => { setModal(''); navigate(`/t/${id}`) }} />}
      {modal === 'pin' && <PinTopic onClose={() => setModal('')} onPin={(pinned, days) => { setModal(''); status({ pinned, pinned_days: days }, 'Pinned') }} />}
      {modal === 'timer' && <TimerTopic t={t} onClose={() => setModal('')} onSave={b => { setModal(''); status(b, 'Saved') }} />}
      {modal === 'email' && <AskText title="Email this topic" label="To" help="One or more addresses, separated by commas. It goes from your own mail account." placeholder="ana@example.com" ok="Send" onClose={() => setModal('')} onDone={v => { setModal(''); if (v.trim()) share('/apps/email', { to: v.split(',').map(a => ({ addr: a.trim() })) }) }} />}
      {modal === 'delete' && <Confirm title="Remove this topic?" message="It disappears for everyone but the moderators, who can bring it back." confirmLabel="Remove" onCancel={() => setModal('')}
        onConfirm={async () => { setModal(''); try { await api.del(`${V1}/topics/${tid}`); toast('Removed'); navigate(`/f/${doc.forum.slug}`); announceChange() } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function JoinHint({ forum, join, onJoined }: { forum: string; join: string; onJoined: () => void }) {
  const { toast } = useToast()
  const go = async () => { try { await api.post<ForumDoc>(`${V1}/forums/${forum}/join`, {}); announceChange(); onJoined() } catch (e) { toast(errMsg(e), 'err') } }
  if (join === 'open') return <div className="reply-start join"><span className="grow">Join the forum to reply.</span><button type="button" className="btn primary" onClick={go}>Join</button></div>
  if (join === 'pending') return <p className="muted center small">Your request to join is waiting for a moderator.</p>
  if (join === 'approval') return <p className="muted center small"><Link to={`/f/${forum}`}>Ask to join the forum</Link> to reply.</p>
  return <p className="muted center small">This forum takes new members by invitation.</p>
}

function EventCard({ t, rsvps, canAnswer, me, onChanged, onCalendar }: { t: Topic; rsvps: RSVP[]; canAnswer: boolean; me: number; onChanged: () => void; onCalendar?: () => void }) {
  const { toast } = useToast()
  const mine = rsvps.find(r => r.user_id === me)?.status || ''
  const going = rsvps.filter(r => r.status === 'going'), maybe = rsvps.filter(r => r.status === 'maybe'), no = rsvps.filter(r => r.status === 'no')
  const p = eventParts(t.event_start)
  const past = t.event_start.slice(0, 10) < new Date().toISOString().slice(0, 10)
  const answer = async (status: string) => { try { await api.post(`${V1}/topics/${t.id}/rsvp`, { status: status === mine ? '' : status }); onChanged() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <section className="card event-card">
      <span className="ev-badge xl"><b>{p.day}</b><small>{p.month}</small></span>
      <div className="grow stack tight">
        <b>{eventWhen(t.event_start, t.event_end)}</b>
        {t.event_place && <span className="muted"><Icon name="pin" size={13} /> {t.event_place}</span>}
        <span className="muted small">{going.length} going{going.length ? `: ${going.map(r => r.name.split(' ')[0]).join(', ')}` : ''}{maybe.length ? ` · ${maybe.length} maybe` : ''}{no.length ? ` · ${no.length} can't` : ''}</span>
      </div>
      <div className="ev-actions">
        {canAnswer && !past && <Segmented size="sm" value={mine || null} onChange={answer} options={[{ value: 'going', label: 'Going' }, { value: 'maybe', label: 'Maybe' }, { value: 'no', label: "Can't" }]} label="Are you coming?" />}
        {onCalendar && <button type="button" className="btn sm" onClick={onCalendar}><Icon name="calendar-plus" size={14} />Calendar</button>}
      </div>
    </section>
  )
}

function EditTopic({ t, slug, mod, onClose, onSaved }: { t: Topic; slug: string; mod: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [title, setTitle] = useState(t.title)
  const [tags, setTags] = useState(t.tags)
  const [kind, setKind] = useState(t.kind)
  const [cat, setCat] = useState(String(t.category_id || ''))
  const [start, setStart] = useState(t.event_start)
  const [place, setPlace] = useState(t.event_place)
  const [cats, setCats] = useState<Category[]>([])
  useEffect(() => { api.get<Category[]>(`${V1}/forums/${slug}/categories`).then(setCats).catch(() => {}) }, [slug])
  const save = async () => {
    try { await api.put(`${V1}/topics/${t.id}`, { title, tags, kind, category_id: Number(cat) || 0, event_start: start, event_place: place }); onSaved() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title="Change the topic" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label="Title"><input value={title} onChange={e => setTitle(e.target.value)} maxLength={200} autoFocus /></Field>
        <Field label="Category" group><Select ariaLabel="Category" value={cat} onChange={setCat} options={[{ value: '', label: 'No category' }, ...cats.filter(c => !c.archived && (mod || c.starts === 'all')).map(c => ({ value: String(c.id), label: `${c.emoji ? c.emoji + ' ' : ''}${c.name}` }))]} /></Field>
        <Field label="Kind" group><Segmented size="sm" value={kind} onChange={setKind} options={KINDS.filter(k => mod || k.value !== 'announcement').map(k => ({ value: k.value, label: k.label }))} /></Field>
        {kind === 'event' && <div className="grid two"><Field label="When"><input type="datetime-local" value={start.length === 10 ? start + 'T00:00' : start} onChange={e => setStart(e.target.value)} /></Field><Field label="Where"><input value={place} onChange={e => setPlace(e.target.value)} /></Field></div>}
        <Field label="Tags" group><TagInput value={tags} onChange={setTags} forum={slug} /></Field>
      </div>
    </Modal>
  )
}

function MoveTopic({ t, slug, onClose, onSaved }: { t: Topic; slug: string; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [cats, setCats] = useState<Category[] | null>(null)
  const [cat, setCat] = useState(String(t.category_id || ''))
  useEffect(() => { api.get<Category[]>(`${V1}/forums/${slug}/categories`).then(setCats).catch(() => setCats([])) }, [slug])
  const save = async () => { try { await api.put(`${V1}/topics/${t.id}`, { category_id: Number(cat) || 0 }); toast('Moved'); onSaved() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Move to another category" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Move</button></>}>
      {!cats ? <Loading /> : <Select ariaLabel="Category" value={cat} onChange={setCat} options={[{ value: '', label: 'No category' }, ...cats.map(c => ({ value: String(c.id), label: `${c.emoji ? c.emoji + ' ' : ''}${c.name}` }))]} />}
    </Modal>
  )
}

function useTopicSearch(slug: string, except: number) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [hits, setHits] = useState<Hit[]>([])
  useEffect(() => {
    if (dq.trim().length < 2) { setHits([]); return }
    api.get<{ hits: Hit[] }>(`${V1}/search?forum=${slug}&titles=1&q=${encodeURIComponent(dq)}`).then(r => { const seen = new Set<number>(); setHits(r.hits.filter(h => h.topic_id !== except && !seen.has(h.topic_id) && seen.add(h.topic_id))) }).catch(() => setHits([]))
  }, [dq, slug, except])
  return { q, setQ, hits }
}

function MergeTopic({ t, slug, onClose, onDone }: { t: Topic; slug: string; onClose: () => void; onDone: (id: number) => void }) {
  const { toast } = useToast()
  const { q, setQ, hits } = useTopicSearch(slug, t.id)
  const [into, setInto] = useState<Hit | null>(null)
  const go = async () => { if (!into) return; try { const r = await api.post<{ topic_id: number }>(`${V1}/topics/${t.id}/merge`, { into: into.topic_id }); toast('Merged'); onDone(r.topic_id) } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Merge into another topic" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!into} onClick={go}>Merge</button></>}>
      <div className="stack">
        <p className="muted small">Every post of “{t.title}” moves to the end of the topic you pick, in the order they were written. This topic then points there.</p>
        <div className="add-search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find the topic by its title" autoFocus /></div>
        <div className="rows">{hits.map(h => <button key={h.topic_id} type="button" className={`row ${into?.topic_id === h.topic_id ? 'on' : ''}`} onClick={() => setInto(h)}><span className="grow"><b>{h.title}</b><span className="muted small">{h.posts_n} posts</span></span>{into?.topic_id === h.topic_id && <Icon name="check" size={16} />}</button>)}</div>
      </div>
    </Modal>
  )
}

function SplitButton({ tid, ids, doc, onDone }: { tid: number; ids: number[]; doc: TopicDoc; onDone: (id: number) => void }) {
  const { toast } = useToast()
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [into, setInto] = useState<Hit | null>(null)
  const { q, setQ, hits } = useTopicSearch(doc.forum.slug, tid)
  const [where, setWhere] = useState<'new' | 'old'>('new')
  const go = async () => {
    try { const r = await api.post<{ topic_id: number }>(`${V1}/topics/${tid}/split`, { post_ids: ids, title, into: where === 'old' ? into?.topic_id : 0 }); toast('Moved'); onDone(r.topic_id) } catch (e) { toast(errMsg(e), 'err') }
  }
  return <>
    <button type="button" className="btn primary" disabled={!ids.length} onClick={() => setOpen(true)}>Move them…</button>
    {open && <Modal title={`Move ${ids.length} ${ids.length === 1 ? 'post' : 'posts'}`} onClose={() => setOpen(false)} footer={<><button type="button" className="btn" onClick={() => setOpen(false)}>Cancel</button><button type="button" className="btn primary" disabled={where === 'new' ? title.trim().length < 3 : !into} onClick={go}>Move</button></>}>
      <div className="stack">
        <Segmented value={where} onChange={setWhere} options={[{ value: 'new', label: 'To a new topic' }, { value: 'old', label: 'To another topic' }]} />
        {where === 'new' ? <Field label="The new topic's title"><input value={title} onChange={e => setTitle(e.target.value)} autoFocus /></Field> : <>
          <div className="add-search"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find the topic by its title" autoFocus /></div>
          <div className="rows">{hits.map(h => <button key={h.topic_id} type="button" className={`row ${into?.topic_id === h.topic_id ? 'on' : ''}`} onClick={() => setInto(h)}><span className="grow"><b>{h.title}</b></span>{into?.topic_id === h.topic_id && <Icon name="check" size={16} />}</button>)}</div>
        </>}
      </div>
    </Modal>}
  </>
}

function PinTopic({ onClose, onPin }: { onClose: () => void; onPin: (pinned: number, days: number) => void }) {
  const [where, setWhere] = useState(2)
  const [days, setDays] = useState(0)
  return (
    <Modal title="Pin the topic" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => onPin(where, days)}>Pin</button></>}>
      <div className="stack">
        <Field label="Where" group><Segmented size="sm" value={where} onChange={setWhere} options={[{ value: 2, label: 'On the front page' }, { value: 1, label: 'In its category' }]} /></Field>
        <Field label="For" group><Segmented size="sm" value={days} onChange={setDays} options={[{ value: 1, label: 'A day' }, { value: 7, label: 'A week' }, { value: 30, label: 'A month' }, { value: 0, label: 'Until unpinned' }]} /></Field>
      </div>
    </Modal>
  )
}

function TimerTopic({ t, onClose, onSave }: { t: Topic; onClose: () => void; onSave: (b: object) => void }) {
  const [close, setClose] = useState(t.close_at ? -1 : 0)
  const [slow, setSlow] = useState(t.slow_mode)
  const options = useMemo(() => [{ value: 0, label: 'Never' }, { value: 1, label: 'In a day' }, { value: 7, label: 'In a week' }, { value: 30, label: 'In a month' }, ...(t.close_at ? [{ value: -1, label: 'As set' }] : [])], [t.close_at])
  return (
    <Modal title="Close later, slow mode" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => onSave({ ...(close >= 0 ? { close_in_days: close } : {}), slow_mode: slow })}>Save</button></>}>
      <div className="stack">
        <Field label="Close it" group help={t.close_at ? `It closes ${new Date(t.close_at).toLocaleString()}.` : 'No more replies after that.'}><Segmented size="sm" value={close} onChange={setClose} options={options} /></Field>
        <Field label="Slow mode" group help="How long each person waits between two replies here. Calms a heated topic."><Segmented size="sm" value={slow} onChange={setSlow} options={[{ value: 0, label: 'Off' }, { value: 60, label: '1 min' }, { value: 600, label: '10 min' }, { value: 3600, label: '1 hour' }, { value: 86400, label: 'A day' }]} /></Field>
      </div>
    </Modal>
  )
}
