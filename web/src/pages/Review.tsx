// The review queue for moderators: posts held back (guests, new members,
// watched words), posts people flagged, and people asking to join.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { useLive } from '../lib/live'
import { ago } from '../lib/format'
import { FLAG_REASONS } from '../lib/forum'
import type { Flag, ForumMini, Member, Post } from '../lib/types'
import { Icon } from '../components/icons'
import { Face } from '../components/forum'
import { Markdown } from '../components/Markdown'
import { Empty, Loading, Spinner, Tabs, errMsg, useToast } from '../components/ui'

interface Queue {
  held: { post: Post; topic_title: string; guest: boolean; guest_email: string; new_topic: boolean; why: string }[]
  flagged: { post: Post; topic_title: string; flags: Flag[] }[]
  joins: { member: Member; forum_id: number }[]
  forums: Record<string, ForumMini>
  moderator: boolean
}

const reasonWord = (r: string) => FLAG_REASONS.find(x => x.value === r)?.label || r

export default function Review() {
  const { toast } = useToast()
  const [q, setQ] = useState<Queue | null>(null)
  const [tab, setTab] = useState<'held' | 'flagged' | 'joins'>('held')
  const [busy, setBusy] = useState('')
  const [ai, setAi] = useState<Record<number, { verdict: string; reason: string } | 'busy'>>({})
  const load = useCallback(() => { api.get<Queue>(`${V1}/review`).then(d => { setQ(d); setTab(t => t === 'held' && !d.held.length ? (d.flagged.length ? 'flagged' : d.joins.length ? 'joins' : 'held') : t) }).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  useLive(e => { if (e.type === 'review') load() })
  const act = async (key: string, fn: () => Promise<unknown>, ok: string) => {
    setBusy(key)
    try { await fn(); toast(ok); load(); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const check = async (postId: number) => {
    setAi(a => ({ ...a, [postId]: 'busy' }))
    try { const r = await api.post<{ verdict: string; reason: string }>(`${V1}/ai/check`, { post_id: postId }); setAi(a => ({ ...a, [postId]: r })) } catch (e) { toast(errMsg(e), 'err'); setAi(a => { const n = { ...a }; delete n[postId]; return n }) }
  }
  if (!q) return <Loading />
  if (!q.moderator) return <div className="page narrow"><Empty title="Nothing to review" icon="shield">The review queue is for the people who moderate a forum.</Empty></div>
  const forumOf = (id: number) => q.forums[id]
  const aiBox = (pid: number) => {
    const r = ai[pid]
    if (!r) return <button type="button" className="btn ghost sm" onClick={() => check(pid)}><Icon name="sparkles" size={14} />Second opinion</button>
    if (r === 'busy') return <span className="muted small"><Spinner size={12} /> reading…</span>
    return <span className={`ai-verdict v-${r.verdict}`}><Icon name="sparkles" size={12} /> {r.verdict === 'fine' ? 'Looks fine' : r.verdict === 'remove' ? 'Should go' : 'Borderline'} — {r.reason}</span>
  }
  return (
    <div className="page narrow">
      <div className="page-head"><div className="grow"><h1 className="page-title">Review</h1><p className="muted small">What waits for a moderator in the forums you look after.</p></div></div>
      <Tabs tabs={[{ id: 'held', label: `Held posts (${q.held.length})` }, { id: 'flagged', label: `Flagged (${q.flagged.length})` }, { id: 'joins', label: `Asking to join (${q.joins.length})` }]} value={tab} onChange={setTab} />
      {tab === 'held' && (q.held.length === 0 ? <div className="card"><Empty title="Nothing held back" icon="check-circle" /></div> : q.held.map(h => (
        <div key={h.post.id} className="card review-card">
          <div className="review-head"><Face name={h.post.author_name} size={30} /><span className="grow"><b>{h.post.author_name}</b>{h.guest && h.guest_email && <span className="muted small"> · {h.guest_email}</span>}
            <span className="muted small"> · {h.new_topic ? 'new topic' : 'reply in'} <Link to={`/t/${h.post.topic_id}/${h.post.number}`}>{h.topic_title}</Link> · {forumOf(h.post.forum_id)?.emoji} {forumOf(h.post.forum_id)?.name} · {ago(h.post.created_at)}</span></span></div>
          <p className="muted small"><Icon name="info" size={12} /> {h.why}</p>
          <div className="review-body"><Markdown text={h.post.body} /></div>
          <div className="row wrap">{aiBox(h.post.id)}<span className="grow" />
            <button type="button" className="btn" disabled={!!busy} onClick={() => act('r' + h.post.id, () => api.post(`${V1}/posts/${h.post.id}/review`, { action: 'reject' }), 'Turned down')}><Icon name="x" size={15} />Turn down</button>
            <button type="button" className="btn primary" disabled={!!busy} onClick={() => act('a' + h.post.id, () => api.post(`${V1}/posts/${h.post.id}/review`, { action: 'approve' }), 'Approved — it is up')}>{busy === 'a' + h.post.id ? <Spinner size={14} /> : <Icon name="check" size={15} />}Approve</button></div>
        </div>
      )))}
      {tab === 'flagged' && (q.flagged.length === 0 ? <div className="card"><Empty title="No flags" icon="check-circle" /></div> : q.flagged.map(f => (
        <div key={f.post.id} className="card review-card">
          <div className="review-head"><Face name={f.post.author_name} color={f.post.color} avatarId={f.post.avatar_id} size={30} /><span className="grow"><b>{f.post.author_name}</b>
            <span className="muted small"> · <Link to={`/t/${f.post.topic_id}/${f.post.number}`}>{f.topic_title}</Link> #{f.post.number} · {forumOf(f.post.forum_id)?.emoji} {forumOf(f.post.forum_id)?.name}</span></span>
            <span className="flag-count">{f.flags.length} {f.flags.length === 1 ? 'flag' : 'flags'}</span></div>
          <div className="stack tight">{f.flags.map(x => <span key={x.id} className="small"><Icon name="flag" size={12} /> <b>{reasonWord(x.reason)}</b> — {x.flagger || 'someone'}{x.note ? `: “${x.note}”` : ''} · {ago(x.created_at)}</span>)}</div>
          <div className="review-body"><Markdown text={f.post.body} /></div>
          <div className="row wrap">{aiBox(f.post.id)}<span className="grow" />
            <button type="button" className="btn" disabled={!!busy} onClick={() => act('k' + f.post.id, () => api.post(`${V1}/posts/${f.post.id}/flags`, { action: 'disagree' }), 'Kept')}>Keep it</button>
            <button type="button" className="btn" disabled={!!busy} onClick={() => act('h' + f.post.id, () => api.post(`${V1}/posts/${f.post.id}/flags`, { action: 'agree' }), 'Hidden')}><Icon name="hide" size={15} />Hide it</button>
            <button type="button" className="btn danger" disabled={!!busy} onClick={() => act('d' + f.post.id, () => api.post(`${V1}/posts/${f.post.id}/flags`, { action: 'delete' }), 'Removed')}><Icon name="trash" size={15} />Remove</button></div>
        </div>
      )))}
      {tab === 'joins' && (q.joins.length === 0 ? <div className="card"><Empty title="Nobody is waiting" icon="users" /></div> : (
        <div className="card flush-card"><div className="rows">{q.joins.map(j => {
          const f = forumOf(j.forum_id)
          return (
            <div key={j.forum_id + ':' + j.member.user_id} className="row">
              <Face name={j.member.name} color={j.member.color} size={34} />
              <span className="grow"><b>{j.member.name}</b><span className="muted small">wants to join {f?.emoji} {f?.name} · {ago(j.member.joined_at)}</span>{j.member.note && <span className="small">“{j.member.note}”</span>}</span>
              <button type="button" className="btn" disabled={!!busy} onClick={() => act('n' + j.member.user_id, () => api.put(`${V1}/forums/${f?.slug}/members/${j.member.user_id}`, { approve: false }), 'Turned down')}>Turn down</button>
              <button type="button" className="btn primary" disabled={!!busy} onClick={() => act('y' + j.member.user_id, () => api.put(`${V1}/forums/${f?.slug}/members/${j.member.user_id}`, { approve: true }), `${j.member.name} is in`)}>Let in</button>
            </div>
          )
        })}</div></div>
      ))}
    </div>
  )
}
