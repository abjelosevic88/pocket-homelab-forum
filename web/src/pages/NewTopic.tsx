// Starting a topic: the forum and category, what kind it is (a discussion,
// a question, an event, an announcement), the title — with similar topics
// shown as you type it — the post, tags, and optionally a poll.
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange } from '../lib/data'
import { KINDS } from '../lib/forum'
import type { Category, ForumBrief, ForumDoc, Hit, TopicKind } from '../lib/types'
import { Icon } from '../components/icons'
import { Composer } from '../components/Composer'
import { useCrumb } from '../components/Layout'
import { Empty, Field, Loading, Segmented, Spinner, Switch, errMsg, useDebounce, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { plain } from '../components/Markdown'

export function TagInput({ value, onChange, forum }: { value: string[]; onChange: (v: string[]) => void; forum: string }) {
  const [text, setText] = useState('')
  const [known, setKnown] = useState<string[]>([])
  useEffect(() => { if (forum) api.get<{ tag: string }[]>(`${V1}/forums/${forum}/tags`).then(t => setKnown(t.map(x => x.tag))).catch(() => {}) }, [forum])
  const add = (t: string) => {
    t = t.trim().replace(/^#/, '').toLowerCase().replace(/\s+/g, '-')
    if (t && !value.includes(t) && value.length < 8) onChange([...value, t])
    setText('')
  }
  const sugg = text ? known.filter(k => k.toLowerCase().startsWith(text.toLowerCase()) && !value.includes(k)).slice(0, 6) : []
  return (
    <div className="tag-input">
      <div className="to-list">
        {value.map(t => <span key={t} className="chip on">#{t}<button type="button" className="chip-x" aria-label={`Remove ${t}`} onClick={() => onChange(value.filter(x => x !== t))}><Icon name="x" size={12} /></button></span>)}
        <input value={text} onChange={e => setText(e.target.value)} placeholder={value.length ? '' : 'Add a tag and press Enter'} aria-label="Tags"
          onKeyDown={e => { if (e.key === 'Enter' || e.key === ',' || e.key === ' ') { if (text.trim()) { e.preventDefault(); add(text) } } else if (e.key === 'Backspace' && !text && value.length) onChange(value.slice(0, -1)) }} onBlur={() => text && add(text)} />
      </div>
      {sugg.length > 0 && <div className="chips mt-s">{sugg.map(s => <button key={s} type="button" className="chip" onMouseDown={e => { e.preventDefault(); add(s) }}>#{s}</button>)}</div>}
    </div>
  )
}

export default function NewTopic() {
  const [sp] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  useCrumb(undefined)
  const [forums, setForums] = useState<ForumBrief[] | null>(null)
  const [forum, setForum] = useState(sp.get('forum') || '')
  const [doc, setDoc] = useState<ForumDoc | null>(null)
  const [cat, setCat] = useState('')
  const [kind, setKind] = useState<TopicKind>('discussion')
  const [kindTouched, setKindTouched] = useState(false)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [place, setPlace] = useState('')
  const [poll, setPoll] = useState(false)
  const [pq, setPq] = useState('')
  const [opts, setOpts] = useState(['', ''])
  const [multi, setMulti] = useState(false)
  const [pub, setPub] = useState(false)
  const [pin, setPin] = useState(false)
  const [busy, setBusy] = useState(false)
  const [aiBusy, setAiBusy] = useState(false)
  const [aiWhy, setAiWhy] = useState('')
  const dTitle = useDebounce(title, 400)
  const [similar, setSimilar] = useState<Hit[]>([])

  useEffect(() => {
    api.get<{ forums: ForumBrief[] }>(`${V1}/forums`).then(r => {
      const mine = r.forums.filter(f => f.status === 'active' && !f.archived)
      setForums(mine)
      if (!forum && mine.length) setForum(mine[0].slug)
    }).catch(() => setForums([]))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!forum) return
    setDoc(null)
    api.get<ForumDoc>(`${V1}/forums/${forum}`).then(d => {
      setDoc(d)
      const want = sp.get('category')
      const c = d.categories.find(x => x.slug === want) || null
      setCat(c ? String(c.id) : '')
      // a draft for this forum
      api.get<{ data: { title?: string; body?: string; category_id?: number; tags?: string[]; kind?: TopicKind } | null }>(`${V1}/drafts/new:${d.forum.id}`).then(r => {
        if (r.data && (r.data.title || r.data.body)) {
          setTitle(t => t || r.data!.title || ''); setBody(b => b || r.data!.body || '')
          if (r.data.category_id && !c) setCat(String(r.data.category_id))
          if (r.data.tags) setTags(r.data.tags)
          if (r.data.kind) { setKind(r.data.kind); setKindTouched(true) }
        }
      }).catch(() => {})
    }).catch(e => toast(errMsg(e), 'err'))
  }, [forum]) // eslint-disable-line react-hooks/exhaustive-deps
  const cats = useMemo(() => (doc?.categories || []).filter(c => !c.archived && (doc?.can.moderate || c.starts === 'all')), [doc])
  const category: Category | undefined = cats.find(c => String(c.id) === cat)
  useEffect(() => { if (!kindTouched) setKind(category?.qa ? 'question' : 'discussion') }, [category, kindTouched])
  useEffect(() => {
    if (dTitle.trim().length < 6 || !forum) { setSimilar([]); return }
    api.get<Hit[]>(`${V1}/similar?forum=${forum}&title=${encodeURIComponent(dTitle)}`).then(setSimilar).catch(() => setSimilar([]))
  }, [dTitle, forum])

  const suggest = async () => {
    setAiBusy(true)
    try {
      const r = await api.post<{ title: string; category_id: number; tags: string[]; kind: TopicKind; why: string }>(`${V1}/ai/suggest`, { forum, title, body })
      if (r.title) setTitle(r.title)
      if (r.category_id) setCat(String(r.category_id))
      if (r.tags?.length) setTags(Array.from(new Set([...tags, ...r.tags])))
      if (r.kind && !kindTouched) setKind(r.kind)
      setAiWhy(r.why)
    } catch (e) { toast(errMsg(e), 'err') } finally { setAiBusy(false) }
  }
  const post = async () => {
    if (!doc) return
    setBusy(true)
    try {
      const r = await api.post<{ topic: { id: number }; state: string }>(`${V1}/topics`, {
        forum, category_id: Number(cat) || 0, title, body, kind, tags, draft_key: `new:${doc.forum.id}`,
        event_start: kind === 'event' ? start : '', event_end: kind === 'event' ? end : '', event_place: kind === 'event' ? place : '',
        poll: poll ? { question: pq, options: opts.filter(o => o.trim()), multi, public: pub } : null, pinned: pin ? 2 : 0,
      })
      announceChange()
      toast(r.state === 'pending' ? 'Sent — a moderator will look at it before it shows' : 'Posted')
      navigate(`/t/${r.topic.id}`, { replace: true })
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }

  if (!forums) return <Loading />
  if (forums.length === 0) return <div className="page narrow"><Empty title="You are not in a forum yet" icon="forum"><Link to="/forums" className="btn primary">Make or join a forum</Link></Empty></div>
  const ready = title.trim().length >= 3 && body.trim().length >= 2 && (kind !== 'event' || !!start) && (!poll || (pq.trim() && opts.filter(o => o.trim()).length >= 2))
  return (
    <div className="page narrow new-topic">
      <h1 className="page-title">New topic</h1>
      <div className="grid two">
        <Field label="Forum" group><Select ariaLabel="Forum" value={forum} onChange={setForum} options={forums.map(f => ({ value: f.slug, label: `${f.emoji} ${f.name}` }))} /></Field>
        <Field label="Category" group>{doc ? <Select ariaLabel="Category" value={cat} onChange={setCat} placeholder="No category" options={[{ value: '', label: 'No category' }, ...cats.map(c => ({ value: String(c.id), label: `${c.emoji ? c.emoji + ' ' : ''}${c.name}`, hint: c.qa ? 'Q&A' : undefined }))]} /> : <Spinner />}</Field>
      </div>
      {category?.about && <p className="muted small cat-about">{category.about}</p>}
      <Field label="What is it?" group>
        <div className="kind-pick">
          {KINDS.filter(k => k.value !== 'announcement' || doc?.can.moderate).map(k => (
            <button key={k.value} type="button" className={`kind-opt ${kind === k.value ? 'on' : ''}`} onClick={() => { setKind(k.value); setKindTouched(true) }} aria-pressed={kind === k.value}>
              <Icon name={k.icon} size={18} /><b>{k.label}</b><small>{k.line}</small>
            </button>
          ))}
        </div>
      </Field>
      <Field label="Title" help={kind === 'question' ? 'Ask it the way someone would search for it.' : undefined}>
        <input value={title} onChange={e => setTitle(e.target.value)} placeholder={kind === 'question' ? 'How do I…?' : kind === 'event' ? 'Summer barbecue at the lake' : 'What is it about?'} maxLength={200} autoFocus className="title-input" />
      </Field>
      {similar.length > 0 && (
        <div className="similar card pad-card">
          <b className="small"><Icon name="search" size={13} /> Maybe it is already here:</b>
          {similar.map(h => <Link key={h.topic_id} to={`/t/${h.topic_id}`} target="_blank" className="similar-row">{h.solved && <Icon name="check-circle" size={13} className="ok" />}<span className="grow ellipsis">{h.title}</span><span className="muted small">{h.posts_n - 1} replies</span></Link>)}
          {similar[0]?.snippet && <span className="muted small ellipsis">{plain(similar[0].snippet, 120)}</span>}
        </div>
      )}
      {kind === 'event' && (
        <div className="grid three">
          <Field label="Starts"><input type="datetime-local" value={start} onChange={e => setStart(e.target.value)} /></Field>
          <Field label="Ends (optional)"><input type="datetime-local" value={end} onChange={e => setEnd(e.target.value)} min={start} /></Field>
          <Field label="Where"><input value={place} onChange={e => setPlace(e.target.value)} placeholder="Address or place" /></Field>
        </div>
      )}
      {doc && <Composer value={body} onChange={setBody} forum={forum} draftKey={`new:${doc.forum.id}`} draftExtra={{ title, category_id: Number(cat) || 0, tags, kind }} minHeight={200} noButton onSubmit={() => { if (ready && !busy) post() }}
        placeholder={kind === 'question' ? 'What did you try, what happened, what did you expect?' : kind === 'event' ? 'What is happening, what to bring, who it is for…' : 'Write your post…'}
        footerLeft={settings?.ai_help !== false && <button type="button" className="btn ghost sm" onClick={suggest} disabled={aiBusy || (!title && !body)} title="The AI suggests a category, tags and a clearer title">{aiBusy ? <Spinner size={13} /> : <Icon name="sparkles" size={14} />}Suggest category & tags</button>} />}
      {aiWhy && <p className="muted small"><Icon name="sparkles" size={12} /> {aiWhy}</p>}
      <Field label="Tags" group><TagInput value={tags} onChange={setTags} forum={forum} /></Field>
      <div className="card pad-card stack">
        <label className="check-row"><Switch checked={poll} onChange={setPoll} label="Add a poll" /><span><b>Add a poll</b><span className="muted small"> — a question with choices to vote on</span></span></label>
        {poll && <>
          <Field label="Question"><input value={pq} onChange={e => setPq(e.target.value)} placeholder="Which day suits you?" /></Field>
          {opts.map((o, i) => <div key={i} className="row"><input value={o} onChange={e => setOpts(opts.map((x, j) => j === i ? e.target.value : x))} placeholder={`Choice ${i + 1}`} />{opts.length > 2 && <button type="button" className="btn ghost icon-only" aria-label="Remove the choice" onClick={() => setOpts(opts.filter((_, j) => j !== i))}><Icon name="x" size={15} /></button>}</div>)}
          {opts.length < 20 && <button type="button" className="btn ghost sm" onClick={() => setOpts([...opts, ''])}><Icon name="plus" size={14} />Another choice</button>}
          <div className="row wrap"><Segmented size="sm" value={multi ? 'multi' : 'one'} onChange={v => setMulti(v === 'multi')} options={[{ value: 'one', label: 'Pick one' }, { value: 'multi', label: 'Pick several' }]} /><label className="check-row"><Switch checked={pub} onChange={setPub} size="sm" label="Show who voted" />Show who voted for what</label></div>
        </>}
        {doc?.can.moderate && <label className="check-row"><Switch checked={pin} onChange={setPin} label="Pin it" /><span><b>Pin it on the front page</b></span></label>}
      </div>
      <div className="row"><span className="grow muted small">{doc && !doc.can.post ? doc.can.why : 'Posting as yourself. ⌘↩ also posts.'}</span>
        <button type="button" className="btn" onClick={() => navigate(-1)}>Cancel</button>
        <button type="button" className="btn primary lg" disabled={!ready || busy || !doc?.can.post} onClick={post}>{busy ? <Spinner size={14} /> : <Icon name="send" size={15} />}Post the topic</button></div>
    </div>
  )
}
