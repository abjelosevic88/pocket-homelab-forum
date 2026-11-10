// One post in a topic: who wrote it, what it says, its reactions and poll,
// and everything you can do with it.
import React, { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtDateTime } from '../lib/format'
import { FLAG_REASONS, LANGS, myLang } from '../lib/forum'
import { openAction, useAppState } from '../lib/data'
import { useApp } from '../lib/app-context'
import type { Poll, Post, Topic, TopicDoc } from '../lib/types'
import { Icon } from './icons'
import { Face, When } from './forum'
import { Markdown } from './Markdown'
import { type Anchor, Field, Loading, Menu, type MenuItem, MenuButton, Modal, Segmented, Spinner, errMsg, rectOf, useToast } from './ui'
import { Select } from './Select'

export interface PostActions {
  reply: (p: Post) => void
  quote: (p: Post) => void
  edit: (p: Post) => void
  changed: () => void
  jump: (n: number) => void
}

export function PostView({ p, doc, act, selectable, selected, onSelect, focused, pub }: {
  p: Post; doc: TopicDoc; act: PostActions; selectable?: boolean; selected?: boolean; onSelect?: (on: boolean) => void; focused?: boolean; pub?: boolean
}) {
  const { toast } = useToast()
  const t = doc.topic
  const [mine, setMine] = useState(p.mine)
  const [counts, setCounts] = useState(p.reactions)
  const [picker, setPicker] = useState<Anchor | null>(null)
  const [modal, setModal] = useState<'' | 'flag' | 'history' | 'bookmark' | 'issue' | 'who'>('')
  const [translated, setTranslated] = useState<{ text: string; by: string } | null>(null)
  const [busy, setBusy] = useState('')
  const appState = useAppState()
  const { user } = useApp()
  React.useEffect(() => { setMine(p.mine); setCounts(p.reactions) }, [p.mine, p.reactions])

  const solved = t.solved_post_id === p.id
  const isOP = p.user_id > 0 && p.user_id === t.user_id
  const can = doc.can
  const own = p.user_id > 0 && p.user_id === user?.id

  const react = async (emoji: string) => {
    setPicker(null)
    const had = mine.includes(emoji)
    setMine(had ? mine.filter(x => x !== emoji) : [...mine, emoji])
    setCounts(c => ({ ...c, [emoji]: Math.max(0, (c[emoji] || 0) + (had ? -1 : 1)) }))
    try { const r = await api.post<{ reactions: Record<string, number>; mine: string[] }>(`${V1}/posts/${p.id}/react`, { emoji }); setCounts(r.reactions); setMine(r.mine) } catch (e) { toast(errMsg(e), 'err'); setMine(p.mine); setCounts(p.reactions) }
  }
  const call = async (what: string, fn: () => Promise<unknown>, ok?: string) => {
    setBusy(what)
    try { await fn(); if (ok) toast(ok); act.changed() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const send = async (path: string, body: object) => {
    setBusy(path)
    try { const r = await api.post<{ text: string; url?: string }>(`${V1}${path}`, body); toast(r.text, 'ok', openAction(r.url)) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }
  const link = `${location.origin}/t/${t.id}/${p.number}`
  const items = (): MenuItem[] => {
    const out: MenuItem[] = []
    if (!pub) {
      out.push({ label: 'Copy link to this post', icon: 'link', onSelect: () => { navigator.clipboard.writeText(link); toast('Link copied') } })
      out.push({ label: p.bookmarked ? 'Bookmarked — change' : 'Bookmark', icon: 'bookmark', onSelect: () => setModal('bookmark') })
      out.push({ label: 'Read it in my language', icon: 'globe', onSelect: () => translate(myLang()) })
      if (p.likes_n > 0) out.push({ label: 'Who reacted', icon: 'heart', onSelect: () => setModal('who') })
      if (p.edits > 0) out.push({ label: `Earlier versions (${p.edits})`, icon: 'history', onSelect: () => setModal('history') })
      out.push('sep')
      const tasks = appState('tasks', 'tasks:write')
      const projects = appState('projects', 'projects:write')
      const learn = appState('learn', 'learn:write')
      const chat = appState('chat', 'chat:write')
      out.push({ label: 'Make a task to follow up', icon: 'check-square', disabled: !tasks.ok, hint: tasks.ok ? undefined : 'Tasks not connected', onSelect: () => send('/apps/tasks', { post_id: p.id }) })
      out.push({ label: 'Make an issue in Pocket Projects', icon: 'layers', disabled: !projects.ok, hint: projects.ok ? undefined : 'not connected', onSelect: () => setModal('issue') })
      out.push({ label: 'Share in Pocket Chat', icon: 'message', disabled: !chat.ok, hint: chat.ok ? undefined : 'not connected', onSelect: () => send('/apps/chat', { post_id: p.id }) })
      if (solved || (t.kind === 'question' && p.number > 1)) out.push({ label: 'Make a flashcard (question → this answer)', icon: 'brain', disabled: !learn.ok, hint: learn.ok ? undefined : 'not connected', onSelect: () => send('/apps/learn', { post_id: p.id }) })
      if (can.solve && p.number > 1 && p.state === 'visible') out.push({ label: solved ? 'Not the solution after all' : 'Mark as the solution', icon: 'check-circle', onSelect: () => call('solve', () => api.post(`${V1}/topics/${t.id}/solve`, { post_id: solved ? 0 : p.id }), solved ? undefined : 'Marked as the solution') })
      const editable = p.state !== 'deleted' && (can.moderate || (p.wiki && can.member) || own)
      if (editable || can.moderate || own) out.push('sep')
      if (editable) out.push({ label: p.wiki ? 'Edit (anyone can)' : 'Edit', icon: 'edit', onSelect: () => act.edit(p) })
      if (own || can.moderate) out.push({ label: p.wiki ? 'Stop letting everyone edit' : 'Let every member edit (wiki)', icon: 'users', onSelect: () => call('wiki', () => api.post(`${V1}/posts/${p.id}/wiki`, { on: !p.wiki })) })
      if (can.moderate && p.state !== 'deleted') out.push({ label: p.state === 'hidden' ? 'Show it again' : 'Hide it', icon: p.state === 'hidden' ? 'eye' : 'hide', onSelect: () => call('hide', () => api.post(`${V1}/posts/${p.id}/hide`, { hidden: p.state !== 'hidden' })) })
      if (p.state === 'deleted' && (can.moderate || own)) out.push({ label: 'Bring it back', icon: 'undo', onSelect: () => call('restore', () => api.post(`${V1}/posts/${p.id}/restore`)) })
      else if (can.moderate || own) out.push({ label: p.number === 1 ? 'Remove the topic' : 'Remove', icon: 'trash', danger: true, onSelect: () => call('delete', () => api.del(`${V1}/posts/${p.id}`), 'Removed') })
      if (!own && can.member) out.push({ label: 'Flag for the moderators', icon: 'flag', onSelect: () => setModal('flag') })
    } else {
      out.push({ label: 'Copy link to this post', icon: 'link', onSelect: () => { navigator.clipboard.writeText(`${location.origin}${location.pathname.replace(/\/t\/\d+.*$/, '')}/t/${t.id}#p${p.number}`); toast('Link copied') } })
    }
    return out
  }
  const translate = async (to: string) => {
    setBusy('tr')
    try { setTranslated(await api.post<{ text: string; by: string }>(`${V1}/apps/translate`, { post_id: p.id, to })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }

  const reactions = Object.entries(counts).filter(([, n]) => n > 0)
  const removed = p.state === 'deleted'
  return (
    <article id={`p${p.number}`} data-n={p.number} className={`post ${solved ? 'solved' : ''} ${p.state !== 'visible' ? 'st-' + p.state : ''} ${focused ? 'focused' : ''} ${selected ? 'selected' : ''}`} aria-label={`Post ${p.number} by ${p.author_name}`}>
      <div className="post-side">
        {selectable && p.number > 1 ? <input type="checkbox" checked={!!selected} onChange={e => onSelect?.(e.target.checked)} aria-label={`Pick post ${p.number}`} className="post-pick" />
          : pub || !p.user_id ? <Face name={p.author_name} color={p.color} avatarId={p.avatar_id} size={40} pub={pub} /> : <Link to={`/u/${p.user_id}`} tabIndex={-1}><Face name={p.author_name} color={p.color} avatarId={p.avatar_id} size={40} /></Link>}
      </div>
      <div className="post-main">
        <header className="post-head">
          {pub || !p.user_id ? <b className="post-author">{p.author_name}</b> : <Link to={`/u/${p.user_id}`} className="post-author">{p.author_name}</Link>}
          {p.handle && <span className="muted small hide-sm">@{p.handle}</span>}
          {p.role === 'owner' && <span className="role-pill owner" title="Runs this forum">Owner</span>}
          {p.role === 'moderator' && <span className="role-pill moderator">Moderator</span>}
          {p.title && <span className="role-pill">{p.title}</span>}
          {isOP && p.number > 1 && <span className="role-pill op" title="Started this topic">Author</span>}
          {p.reply_to > 0 && <button type="button" className="reply-to linkish small" onClick={() => act.jump(p.reply_to)} title={`In reply to #${p.reply_to}`}><Icon name="corner-down-right" size={12} />#{p.reply_to}</button>}
          <span className="grow" />
          {p.wiki && <span className="chip-s" title="Every member can edit this post"><Icon name="users" size={12} />wiki</span>}
          {p.edits > 0 && !pub && <button type="button" className="linkish muted small" onClick={() => setModal('history')} title={p.edited_at ? `Edited ${fmtDateTime(p.edited_at)}` : 'Edited'}><Icon name="edit" size={12} /></button>}
          <a href={`#p${p.number}`} className="post-time muted small" onClick={e => { e.preventDefault(); act.jump(p.number) }}><When at={p.created_at} /></a>
        </header>
        {p.state === 'pending' && <div className="post-note warn"><Icon name="clock" size={14} />Waiting for a moderator — only you{can.moderate ? ' and the moderators' : ''} can see it.</div>}
        {p.state === 'hidden' && <div className="post-note"><Icon name="hide" size={14} />Hidden by a moderator{p.hidden_reason ? `: ${p.hidden_reason}` : ''}.</div>}
        {removed ? <p className="muted post-removed">{p.body ? <><i>Removed.</i> <span className="small">(You can still see it.)</span></> : <i>This post was removed.</i>}</p> : null}
        {(!removed || p.body) && <Markdown text={p.body} topicId={t.id} pub={pub} onQuoteClick={act.jump} className={removed ? 'removed-body' : ''} />}
        {translated && <div className="post-translation"><div className="muted small"><Icon name="globe" size={12} /> Translated by {translated.by} · <button type="button" className="linkish" onClick={() => setTranslated(null)}>hide</button></div><Markdown text={translated.text} topicId={t.id} /></div>}
        {p.poll && <PollView poll={p.poll} pub={pub} canVote={can.member && !pub} canClose={can.moderate || own} />}
        {solved && <div className="solved-mark"><Icon name="check-circle" size={15} />The solution</div>}
        <footer className="post-foot">
          <div className="reacts">
            {reactions.map(([e, n]) => <button key={e} type="button" className={`react ${mine.includes(e) ? 'mine' : ''}`} onClick={() => !pub && can.member && !own && react(e)} disabled={pub || !can.member || own} title={mine.includes(e) ? 'Take your reaction back' : 'React'}>{e}<span>{n}</span></button>)}
            {!pub && can.member && !own && p.state === 'visible' && <button type="button" className="react add" aria-label="React" title="React" onClick={e => setPicker(rectOf(e.currentTarget))}><Icon name="smile" size={15} /></button>}
          </div>
          <span className="grow" />
          {busy && <Spinner size={13} />}
          {p.replies > 0 && <span className="muted small hide-sm">{p.replies === 1 ? '1 reply' : `${p.replies} replies`}</span>}
          {!pub && can.reply && p.state === 'visible' && <>
            <button type="button" className="btn ghost sm" onClick={() => act.quote(p)} title="Quote"><Icon name="quote" size={14} /><span className="hide-sm">Quote</span></button>
            <button type="button" className="btn ghost sm" onClick={() => act.reply(p)} title="Reply to this post"><Icon name="reply" size={15} />Reply</button>
          </>}
          <MenuButton items={items} label="More" className="btn ghost sm icon-only" align="right" width={290}><Icon name="more" size={15} /></MenuButton>
        </footer>
      </div>
      {picker && <Menu anchor={picker} onClose={() => setPicker(null)} width={56 * Math.min(doc.forum.reactions.length, 8) + 12} items={[{ label: <span className="react-pick">{doc.forum.reactions.map(e => <button key={e} type="button" className={`react-big ${mine.includes(e) ? 'mine' : ''}`} onClick={() => react(e)} aria-label={`React with ${e}`}>{e}</button>)}</span> }]} />}
      {modal === 'flag' && <FlagModal post={p} onClose={() => setModal('')} />}
      {modal === 'history' && <HistoryModal post={p} onClose={() => setModal('')} />}
      {modal === 'bookmark' && <BookmarkModal post={p} onClose={() => setModal('')} onDone={act.changed} />}
      {modal === 'issue' && <IssueModal post={p} topic={t} onClose={() => setModal('')} />}
      {modal === 'who' && <WhoModal post={p} onClose={() => setModal('')} />}
    </article>
  )
}

export function PollView({ poll: initial, canVote, canClose, pub }: { poll: Poll; canVote: boolean; canClose: boolean; pub?: boolean }) {
  const { toast } = useToast()
  const [poll, setPoll] = useState(initial)
  const [pick, setPick] = useState<number[]>(initial.mine)
  const [busy, setBusy] = useState(false)
  React.useEffect(() => { setPoll(initial); setPick(initial.mine) }, [initial])
  const closed = poll.closed || (!!poll.closes_at && poll.closes_at <= new Date().toISOString())
  const voted = poll.mine.length > 0
  const show = closed || voted || !canVote
  const total = poll.counts.reduce((a, b) => a + b, 0) || 1
  const max = Math.max(...poll.counts)
  const vote = async (choices: number[]) => {
    setBusy(true)
    try { const p = await api.post<Poll>(`${V1}/polls/${poll.id}/vote`, { choices }); setPoll(p); setPick(p.mine) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const close = async () => {
    try { setPoll(await api.post<Poll>(`${V1}/polls/${poll.id}/close`, { closed: !poll.closed })) } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="poll">
      <div className="poll-q"><Icon name="poll" size={15} /><b>{poll.question}</b></div>
      <div className="poll-opts">
        {poll.options.map((o, i) => {
          const pct = Math.round((poll.counts[i] / total) * 100)
          const on = pick.includes(i)
          return show ? (
            <div key={i} className={`poll-res ${poll.counts[i] === max && max > 0 ? 'lead' : ''} ${poll.mine.includes(i) ? 'mine' : ''}`} title={poll.who?.[i]?.join(', ')}>
              <span className="poll-bar" style={{ width: `${pct}%` }} />
              <span className="poll-label">{poll.mine.includes(i) && <Icon name="check" size={13} />}{o}</span>
              <span className="poll-pct">{pct}%</span>
            </div>
          ) : (
            <label key={i} className={`poll-opt ${on ? 'on' : ''}`}>
              <input type={poll.multi ? 'checkbox' : 'radio'} name={`poll${poll.id}`} checked={on} onChange={() => setPick(poll.multi ? (on ? pick.filter(x => x !== i) : [...pick, i]) : [i])} />{o}
            </label>
          )
        })}
      </div>
      <div className="poll-foot muted small">
        <span>{poll.voters} {poll.voters === 1 ? 'vote' : 'votes'}{poll.multi ? ' · pick several' : ''}{poll.public ? ' · votes are shown' : ''}{closed ? ' · closed' : poll.closes_at ? ` · closes ${fmtDateTime(poll.closes_at)}` : ''}</span>
        <span className="grow" />
        {!show && <button type="button" className="btn primary sm" disabled={!pick.length || busy} onClick={() => vote(pick)}>{busy && <Spinner size={12} />}Vote</button>}
        {show && canVote && !closed && voted && <button type="button" className="linkish" onClick={() => vote([])}>Change my vote</button>}
        {canClose && !pub && <button type="button" className="linkish" onClick={close}>{poll.closed ? 'Reopen' : 'Close the poll'}</button>}
      </div>
    </div>
  )
}

function FlagModal({ post, onClose }: { post: Post; onClose: () => void }) {
  const { toast } = useToast()
  const [reason, setReason] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async () => {
    setBusy(true)
    try { const r = await api.post<{ text: string }>(`${V1}/posts/${post.id}/flag`, { reason, note }); toast(r.text); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Tell the moderators" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!reason || busy || (reason === 'other' && !note.trim())} onClick={go}>Flag it</button></>}>
      <div className="stack">
        {FLAG_REASONS.map(r => <label key={r.value} className={`radio-row ${reason === r.value ? 'on' : ''}`}><input type="radio" name="reason" checked={reason === r.value} onChange={() => setReason(r.value)} /><span><b>{r.label}</b><p className="muted small">{r.line}</p></span></label>)}
        <textarea value={note} onChange={e => setNote(e.target.value)} rows={2} placeholder="Anything the moderators should know (optional)" maxLength={500} />
        <p className="muted small">Only the moderators see who flagged a post.</p>
      </div>
    </Modal>
  )
}

function HistoryModal({ post, onClose }: { post: Post; onClose: () => void }) {
  const [data, setData] = useState<{ current: string; revisions: { id: number; editor: string; title: string; body: string; reason: string; created_at: string }[] } | null>(null)
  const [i, setI] = useState(0)
  React.useEffect(() => { api.get<typeof data>(`${V1}/posts/${post.id}/revisions`).then(setData).catch(() => setData({ current: post.body, revisions: [] })) }, [post.id, post.body])
  if (!data) return <Modal title="Earlier versions" onClose={onClose}><Loading /></Modal>
  const r = data.revisions[i]
  return (
    <Modal title="Earlier versions" onClose={onClose} size="lg">
      {!r ? <p className="muted">No earlier versions.</p> : (
        <div className="stack">
          <div className="row wrap"><Segmented size="sm" value={i} onChange={setI} options={data.revisions.map((x, j) => ({ value: j, label: j === 0 ? 'Before the last edit' : `${j + 1} edits ago` }))} /></div>
          <p className="muted small">Changed by {r.editor || 'someone'} {fmtDateTime(r.created_at)}{r.reason ? ` — “${r.reason}”` : ''}{r.reason === 'title' ? ` (the title was “${r.title}”)` : ''}</p>
          <div className="rev-grid"><div><h4 className="sc-h">Before</h4><div className="rev-box"><Markdown text={r.body} /></div></div><div><h4 className="sc-h">After</h4><div className="rev-box"><Markdown text={i === 0 ? data.current : data.revisions[i - 1].body} /></div></div></div>
        </div>
      )}
    </Modal>
  )
}

function BookmarkModal({ post, onClose, onDone }: { post: Post; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [note, setNote] = useState('')
  const [when, setWhen] = useState<'none' | 'later' | 'tomorrow' | 'week'>('none')
  const save = async () => {
    const d = new Date()
    if (when === 'later') d.setHours(d.getHours() + 3)
    if (when === 'tomorrow') { d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0) }
    if (when === 'week') { d.setDate(d.getDate() + ((8 - d.getDay()) % 7 || 7)); d.setHours(9, 0, 0, 0) }
    try { await api.post(`${V1}/posts/${post.id}/bookmark`, { note, remind_at: when === 'none' ? '' : d.toISOString().replace(/\.\d+Z$/, 'Z') }); toast(when === 'none' ? 'Bookmarked' : 'Bookmarked — you will get a reminder'); onDone(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  const remove = async () => { try { await api.del(`${V1}/posts/${post.id}/bookmark`); toast('Bookmark removed'); onDone(); onClose() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Bookmark this post" onClose={onClose} size="sm" footer={<>{post.bookmarked && <button type="button" className="btn danger" onClick={remove}>Remove</button>}<span className="grow" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label="A note for yourself"><input value={note} onChange={e => setNote(e.target.value)} placeholder="Try this at home" maxLength={300} autoFocus /></Field>
        <Field label="Remind me" group><Segmented size="sm" value={when} onChange={setWhen} options={[{ value: 'none', label: 'No' }, { value: 'later', label: 'In 3 hours' }, { value: 'tomorrow', label: 'Tomorrow' }, { value: 'week', label: 'Next week' }]} /></Field>
      </div>
    </Modal>
  )
}

function IssueModal({ post, topic, onClose }: { post: Post; topic: Topic; onClose: () => void }) {
  const { toast } = useToast()
  const [teams, setTeams] = useState<{ id: string; name: string }[] | null>(null)
  const [team, setTeam] = useState('')
  const [title, setTitle] = useState(topic.title)
  const [busy, setBusy] = useState(false)
  React.useEffect(() => { api.get<{ teams: { id: string; name: string }[] }>(`${V1}/apps/teams`).then(r => { setTeams(r.teams); if (r.teams[0]) setTeam(r.teams[0].id) }).catch(e => { toast(errMsg(e), 'err'); setTeams([]) }) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const go = async () => {
    setBusy(true)
    try { const r = await api.post<{ text: string; url: string }>(`${V1}/apps/projects`, { post_id: post.id, team_id: Number(team), title }); toast(r.text, 'ok', openAction(r.url)); onClose() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Make an issue in Pocket Projects" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!team || busy} onClick={go}>{busy && <Spinner size={14} />}Make the issue</button></>}>
      {!teams ? <Loading /> : <div className="stack">
        <Field label="Title"><input value={title} onChange={e => setTitle(e.target.value)} /></Field>
        <Field label="Team" group><Select ariaLabel="Team" value={team} onChange={setTeam} options={teams.map(t => ({ value: t.id, label: t.name }))} /></Field>
        <p className="muted small">The topic's first post{post.number > 1 ? ' and this reply' : ''} become its description, with a link back here.</p>
      </div>}
    </Modal>
  )
}

function WhoModal({ post, onClose }: { post: Post; onClose: () => void }) {
  const [list, setList] = useState<{ user_id: number; name: string; emoji: string }[] | null>(null)
  React.useEffect(() => { api.get<typeof list>(`${V1}/posts/${post.id}/reactions`).then(setList).catch(() => setList([])) }, [post.id])
  return (
    <Modal title="Who reacted" onClose={onClose} size="sm">
      {!list ? <Loading /> : <div className="rows">{list.map((r, i) => <Link key={i} to={`/u/${r.user_id}`} className="row" onClick={onClose}><span className="react-big static">{r.emoji}</span><span className="grow">{r.name}</span></Link>)}</div>}
    </Modal>
  )
}

export const TRANSLATE_LANGS = LANGS
