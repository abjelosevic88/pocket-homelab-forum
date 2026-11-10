// A forum's own settings, for its owner and moderators: what it is, who may
// see and join it, its categories, its people and invitations, the
// moderation rules, the log, bringing topics in, and the danger zone.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { ago, fmtDateTime } from '../lib/format'
import { COLORS, EMOJIS, JOIN, VISIBILITY } from '../lib/forum'
import type { Category, ForumDoc, Invite, JoinPolicy, LogEntry, Member, Visibility } from '../lib/types'
import { Icon } from '../components/icons'
import { AskText, Face, ForumMark } from '../components/forum'
import { InviteModal } from '../components/Invite'
import { useCrumb } from '../components/Layout'
import { Confirm, CopyField, Empty, Field, Loading, type MenuItem, MenuButton, Modal, Segmented, SettingRow, Spinner, Switch, Tabs, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'

type Tab = 'general' | 'access' | 'categories' | 'people' | 'invites' | 'moderation' | 'log' | 'import'

export default function ForumSettings() {
  const { slug = '', tab: raw } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [doc, setDoc] = useState<ForumDoc | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(() => api.get<ForumDoc>(`${V1}/forums/${slug}`).then(setDoc).catch(e => setErr(errMsg(e))), [slug])
  useEffect(() => { load() }, [load])
  useCrumb(doc ? `${doc.forum.name} — settings` : undefined, `/f/${slug}`)
  if (err) return <div className="page narrow"><Empty title="Not here" icon="lock">{err}</Empty></div>
  if (!doc) return <Loading />
  if (!doc.can.moderate) return <div className="page narrow"><Empty title="Only for the forum's moderators" icon="shield" /></div>
  const own = doc.can.own
  const tabs: { id: Tab; label: string; icon: string }[] = [
    { id: 'general', label: 'General', icon: 'settings' }, ...(own ? [{ id: 'access' as Tab, label: 'Who can join', icon: 'lock' }] : []), { id: 'categories', label: 'Categories', icon: 'layers' },
    { id: 'people', label: 'People', icon: 'users' }, { id: 'invites', label: 'Invitations', icon: 'mail' }, { id: 'moderation', label: 'Moderation', icon: 'shield' },
    { id: 'log', label: 'Log', icon: 'history' }, ...(own ? [{ id: 'import' as Tab, label: 'Bring topics in', icon: 'download' }] : []),
  ]
  const tab: Tab = tabs.find(t => t.id === raw)?.id || 'general'
  const save = async (patch: object, ok = 'Saved') => {
    try { const d = await api.put<ForumDoc>(`${V1}/forums/${slug}`, patch); setDoc(d); toast(ok); announceChange(); if (d.forum.slug !== slug) navigate(`/f/${d.forum.slug}/settings/${tab}`, { replace: true }) } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="page fs-page">
      <div className="page-head"><ForumMark f={doc.forum} size={36} /><div className="grow"><h1 className="page-title">{doc.forum.name}</h1><p className="muted small">Forum settings · <Link to={`/f/${slug}`}>back to the forum</Link></p></div></div>
      <div className="scroll-x"><Tabs tabs={tabs} value={tab} onChange={t => navigate(`/f/${slug}/settings/${t}`, { replace: true })} /></div>
      {tab === 'general' && <General doc={doc} own={own} save={save} />}
      {tab === 'access' && own && <Access doc={doc} save={save} />}
      {tab === 'categories' && <Categories doc={doc} reload={load} />}
      {tab === 'people' && <People doc={doc} />}
      {tab === 'invites' && <Invites doc={doc} />}
      {tab === 'moderation' && <Moderation doc={doc} save={save} />}
      {tab === 'log' && <Log slug={slug} />}
      {tab === 'import' && own && <Import slug={slug} />}
    </div>
  )
}

function General({ doc, own, save }: { doc: ForumDoc; own: boolean; save: (p: object, ok?: string) => Promise<void> }) {
  const f = doc.forum
  const navigate = useNavigate()
  const { toast } = useToast()
  const [name, setName] = useState(f.name)
  const [about, setAbout] = useState(f.about)
  const [emoji, setEmoji] = useState(f.emoji)
  const [color, setColor] = useState(f.color || COLORS[0])
  const [rules, setRules] = useState(f.rules)
  const [welcome, setWelcome] = useState(f.welcome)
  const [slug, setSlug] = useState(f.slug)
  const [del, setDel] = useState(false)
  const [owner, setOwner] = useState('')
  const [members, setMembers] = useState<Member[]>([])
  useEffect(() => { if (own) api.get<{ members: Member[] }>(`${V1}/forums/${f.slug}/members?status=active`).then(r => setMembers(r.members.filter(m => m.role !== 'owner'))).catch(() => {}) }, [own, f.slug])
  return (
    <div className="stack loose fs-body">
      <section className="card pad-card stack">
        <div className="row"><span className="forum-mark big" style={{ background: `color-mix(in srgb, ${color} 16%, transparent)` }}>{emoji}</span><Field label="Name" className="grow"><input value={name} onChange={e => setName(e.target.value)} maxLength={80} /></Field></div>
        <Field label="One line about it"><input value={about} onChange={e => setAbout(e.target.value)} maxLength={300} /></Field>
        <Field label="Picture" group><div className="emoji-pick">{Array.from(new Set([emoji, ...EMOJIS])).map(e => <button key={e} type="button" className={emoji === e ? 'on' : ''} onClick={() => setEmoji(e)}>{e}</button>)}<input className="emoji-own" value={emoji} onChange={e => setEmoji(e.target.value)} maxLength={8} aria-label="Your own emoji" /></div></Field>
        <Field label="Colour" group><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${color === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setColor(c)} aria-label={c} />)}</div></Field>
        <Field label="Rules" help="Markdown. Shown on the About page and when someone asks to join."><textarea value={rules} onChange={e => setRules(e.target.value)} rows={8} /></Field>
        <Field label="Welcome message" help="New members get it as a notification."><textarea value={welcome} onChange={e => setWelcome(e.target.value)} rows={3} /></Field>
        {own && <Field label="Address" help={`${location.origin}/f/${slug}`}><input value={slug} onChange={e => setSlug(e.target.value.toLowerCase())} pattern="[a-z0-9-]+" /></Field>}
        <div className="row"><span className="grow" /><button type="button" className="btn primary" onClick={() => save({ name, about, emoji, color, rules, welcome, ...(own ? { slug } : {}) })}>Save changes</button></div>
      </section>
      {own && <section className="card pad-card stack danger-zone">
        <h3 className="card-h">Danger zone</h3>
        <SettingRow title="Archive" help="Read only for everyone; nothing is lost and you can undo it."><Switch checked={f.archived} onChange={v => save({ archived: v }, v ? 'Archived' : 'Open again')} label="Archived" /></SettingRow>
        <SettingRow title="Hand the forum over" help="Someone else becomes its owner; you stay as a moderator.">
          <div className="row"><Select ariaLabel="New owner" value={owner} onChange={setOwner} placeholder="Pick a member" options={members.map(m => ({ value: String(m.user_id), label: m.name }))} />
            <button type="button" className="btn" disabled={!owner} onClick={() => save({ owner_id: Number(owner) }, 'Handed over')}>Hand over</button></div>
        </SettingRow>
        <SettingRow title="Delete the forum" help="Every topic, post and picture in it goes, for everyone. Export first if you want to keep it (Settings → Backup & data)."><button type="button" className="btn danger" onClick={() => setDel(true)}><Icon name="trash" size={15} />Delete</button></SettingRow>
      </section>}
      {del && <Confirm title={`Delete ${f.name}?`} message="This cannot be undone." confirmLabel="Delete for good" onCancel={() => setDel(false)}
        onConfirm={async () => { try { await api.del(`${V1}/forums/${f.slug}`); announceChange(); toast('Deleted'); navigate('/forums') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function Access({ doc, save }: { doc: ForumDoc; save: (p: object, ok?: string) => Promise<void> }) {
  const f = doc.forum
  return (
    <div className="stack loose fs-body">
      <section className="card pad-card stack">
        <h3 className="card-h">Who can see it</h3>
        {(Object.keys(VISIBILITY) as Visibility[]).map(v => (
          <label key={v} className={`radio-row ${f.visibility === v ? 'on' : ''}`}><input type="radio" name="vis" checked={f.visibility === v} onChange={() => save({ visibility: v })} /><Icon name={VISIBILITY[v].icon} size={16} /><span><b>{VISIBILITY[v].label}</b><p className="muted small">{VISIBILITY[v].line}{v === 'public' ? '. Pocket apps are on your own network unless you put them on the internet yourself.' : ''}</p></span></label>
        ))}
        {f.visibility === 'public' && <><CopyField value={doc.public_url} label="Public link" /><CopyField value={doc.feed_url} label="RSS feed" /></>}
      </section>
      {f.visibility !== 'private' && <section className="card pad-card stack">
        <h3 className="card-h">How people join</h3>
        {(Object.keys(JOIN) as JoinPolicy[]).map(j => (
          <label key={j} className={`radio-row ${f.join_policy === j ? 'on' : ''}`}><input type="radio" name="join" checked={f.join_policy === j} onChange={() => save({ join_policy: j })} /><span><b>{JOIN[j].label}</b><p className="muted small">{JOIN[j].line}</p></span></label>
        ))}
      </section>}
      {f.visibility === 'public' && <section className="card pad-card">
        <SettingRow title="Guests may post" help="People without an account can start topics and reply with their name. Every guest post waits in Review until a moderator approves it.">
          <Switch checked={f.guest_posts} onChange={v => save({ guest_posts: v })} label="Guest posts" /></SettingRow>
      </section>}
    </div>
  )
}

function Categories({ doc, reload }: { doc: ForumDoc; reload: () => void }) {
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<Category> | null>(null)
  const [del, setDel] = useState<Category | null>(null)
  const [moveTo, setMoveTo] = useState('')
  const cats = doc.categories
  const move = async (i: number, d: number) => {
    const ids = cats.map(c => c.id)
    const j = i + d
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    try { await api.put(`${V1}/forums/${doc.forum.slug}/categories/order`, { ids }); reload() } catch (e) { toast(errMsg(e), 'err') }
  }
  const save = async () => {
    if (!edit) return
    try {
      if (edit.id) await api.put(`${V1}/categories/${edit.id}`, edit)
      else await api.post(`${V1}/forums/${doc.forum.slug}/categories`, edit)
      setEdit(null); reload(); toast('Saved')
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="stack fs-body">
      <div className="row"><p className="muted small grow">Topics live in categories. Drag order with the arrows; a category can sit inside another one.</p><button type="button" className="btn primary" onClick={() => setEdit({ name: '', emoji: '', color: COLORS[1], about: '', starts: 'all', qa: false, parent_id: 0 })}><Icon name="plus" size={15} />New category</button></div>
      <div className="card flush-card"><div className="rows">
        {cats.map((c, i) => (
          <div key={c.id} className={`row ${c.archived ? 'dim' : ''}`}>
            <span className="cat-ico sm" style={{ background: `color-mix(in srgb, ${c.color || 'var(--accent)'} 18%, transparent)` }}>{c.emoji || '#'}</span>
            <span className="grow"><b>{c.parent_id ? <span className="muted">↳ </span> : null}{c.name}</b><span className="muted small">{c.topics} topics{c.qa ? ' · questions & answers' : ''}{c.starts === 'mods' ? ' · moderators start topics' : ''}{c.archived ? ' · archived' : ''}</span></span>
            <button type="button" className="btn ghost icon-only sm" aria-label="Up" onClick={() => move(i, -1)} disabled={i === 0}><Icon name="chevron-up" size={15} /></button>
            <button type="button" className="btn ghost icon-only sm" aria-label="Down" onClick={() => move(i, 1)} disabled={i === cats.length - 1}><Icon name="chevron-down" size={15} /></button>
            <button type="button" className="btn sm" onClick={() => setEdit({ ...c })}>Edit</button>
            <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => setDel(c)}><Icon name="trash" size={15} /></button>
          </div>
        ))}
      </div></div>
      {edit && <Modal title={edit.id ? 'Change the category' : 'New category'} onClose={() => setEdit(null)} footer={<><button type="button" className="btn" onClick={() => setEdit(null)}>Cancel</button><button type="button" className="btn primary" disabled={!edit.name?.trim()} onClick={save}>Save</button></>}>
        <div className="stack">
          <div className="row"><Field label="Emoji"><input value={edit.emoji || ''} onChange={e => setEdit({ ...edit, emoji: e.target.value })} maxLength={8} className="emoji-own" /></Field><Field label="Name" className="grow"><input value={edit.name || ''} onChange={e => setEdit({ ...edit, name: e.target.value })} autoFocus maxLength={60} /></Field></div>
          <Field label="What goes in it"><input value={edit.about || ''} onChange={e => setEdit({ ...edit, about: e.target.value })} maxLength={300} /></Field>
          <Field label="Colour" group><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${edit.color === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setEdit({ ...edit, color: c })} aria-label={c} />)}</div></Field>
          <Field label="Inside" group><Select ariaLabel="Parent" value={String(edit.parent_id || '')} onChange={v => setEdit({ ...edit, parent_id: Number(v) || 0 })} options={[{ value: '', label: 'Nothing (a top category)' }, ...cats.filter(c => !c.parent_id && c.id !== edit.id).map(c => ({ value: String(c.id), label: c.name }))]} /></Field>
          <SettingRow title="Questions & answers" help="New topics are questions; the person who asked marks the reply that solved it."><Switch checked={!!edit.qa} onChange={v => setEdit({ ...edit, qa: v })} label="Q&A" /></SettingRow>
          <SettingRow title="Only moderators start topics" help="Everyone can still reply. Good for announcements."><Switch checked={edit.starts === 'mods'} onChange={v => setEdit({ ...edit, starts: v ? 'mods' : 'all' })} label="Moderators only" /></SettingRow>
          {edit.id && <SettingRow title="Archived" help="Hidden from the new-topic list; its topics stay readable."><Switch checked={!!edit.archived} onChange={v => setEdit({ ...edit, archived: v })} label="Archived" /></SettingRow>}
        </div>
      </Modal>}
      {del && <Modal title={`Remove ${del.name}?`} onClose={() => setDel(null)} size="sm" footer={<><button type="button" className="btn" onClick={() => setDel(null)}>Cancel</button>
        <button type="button" className="btn danger" onClick={async () => { try { await api.del(`${V1}/categories/${del.id}?move_to=${moveTo || 0}`); setDel(null); reload(); toast('Removed') } catch (e) { toast(errMsg(e), 'err') } }}>Remove</button></>}>
        <div className="stack"><p className="muted">Its {del.topics} topics move to:</p><Select ariaLabel="Move to" value={moveTo} onChange={setMoveTo} options={[{ value: '', label: 'No category' }, ...cats.filter(c => c.id !== del.id).map(c => ({ value: String(c.id), label: c.name }))]} /></div>
      </Modal>}
    </div>
  )
}

function People({ doc }: { doc: ForumDoc }) {
  const { toast } = useToast()
  const [status, setStatus] = useState<'active' | 'pending' | 'banned'>('active')
  const [q, setQ] = useState('')
  const [list, setList] = useState<Member[] | null>(null)
  const [ban, setBan] = useState<Member | null>(null)
  const [invite, setInvite] = useState(false)
  const [titled, setTitled] = useState<Member | null>(null)
  const slug = doc.forum.slug
  const load = useCallback(() => api.get<{ members: Member[] }>(`${V1}/forums/${slug}/members?status=${status}&q=${encodeURIComponent(q)}`).then(r => setList(r.members)).catch(() => setList([])), [slug, status, q])
  useEffect(() => { const t = window.setTimeout(load, 200); return () => window.clearTimeout(t) }, [load])
  const put = async (m: Member, body: object, ok: string) => { try { await api.put(`${V1}/forums/${slug}/members/${m.user_id}`, body); toast(ok); load() } catch (e) { toast(errMsg(e), 'err') } }
  const items = (m: Member): MenuItem[] => m.role === 'owner' ? [{ label: 'The owner', disabled: true }] : [
    ...(doc.can.own ? [{ label: m.role === 'moderator' ? 'Make a member again' : 'Make a moderator', icon: 'shield', onSelect: () => put(m, { role: m.role === 'moderator' ? 'member' : 'moderator' }, 'Saved') }] : []),
    { label: 'Give a title…', icon: 'tag', onSelect: () => setTitled(m) },
    'sep',
    { label: m.silenced_until ? 'Let them post again' : 'Pause their posting for a day', icon: 'clock', onSelect: () => put(m, { silence_hours: m.silenced_until ? 0 : 24 }, m.silenced_until ? 'They can post again' : 'Paused for a day') },
    { label: 'Ban…', icon: 'x-circle', danger: true, onSelect: () => setBan(m) },
    { label: 'Remove from the forum', icon: 'log-out', danger: true, onSelect: async () => { try { await api.del(`${V1}/forums/${slug}/members/${m.user_id}`); toast('Removed'); load() } catch (e) { toast(errMsg(e), 'err') } } },
  ]
  return (
    <div className="stack fs-body">
      <div className="toolbar row wrap"><Segmented value={status} onChange={setStatus} options={[{ value: 'active', label: 'Members' }, { value: 'pending', label: 'Asking to join' }, { value: 'banned', label: 'Banned' }]} />
        <div className="add-search grow"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find someone" /></div>
        <button type="button" className="btn primary" onClick={() => setInvite(true)}><Icon name="plus" size={15} />Invite</button></div>
      <div className="card flush-card">
        {!list ? <Loading /> : list.length === 0 ? <Empty title="Nobody here" icon="users" /> : <div className="rows">{list.map(m => (
          <div key={m.user_id} className="row">
            <Face name={m.name} color={m.color} avatarId={m.avatar_id} size={34} />
            <span className="grow"><b><Link to={`/u/${m.user_id}`}>{m.name}</Link> <span className="muted small">@{m.handle}</span>{m.role !== 'member' && <span className={`role-pill ${m.role}`}>{m.role === 'owner' ? 'Owner' : 'Moderator'}</span>}{m.title && <span className="role-pill">{m.title}</span>}</b>
              <span className="muted small">{m.posts_n} posts · joined {ago(m.joined_at)}{m.last_seen_at ? ` · seen ${ago(m.last_seen_at)}` : ''}{m.silenced_until ? ` · paused until ${fmtDateTime(m.silenced_until)}` : ''}</span>
              {m.status === 'pending' && m.note && <span className="small">“{m.note}”</span>}
              {m.status === 'banned' && <span className="small danger-text">Banned{m.banned_until ? ` until ${fmtDateTime(m.banned_until)}` : ''}{m.ban_reason ? `: ${m.ban_reason}` : ''}</span>}</span>
            {m.status === 'pending' ? <><button type="button" className="btn sm" onClick={() => put(m, { approve: false }, 'Turned down')}>Turn down</button><button type="button" className="btn primary sm" onClick={() => put(m, { approve: true }, `${m.name} is in`)}>Let in</button></>
              : m.status === 'banned' ? <button type="button" className="btn sm" onClick={() => put(m, { status: 'active' }, 'Ban lifted')}>Lift the ban</button>
                : <MenuButton items={() => items(m)} label="More" className="btn ghost icon-only sm" align="right" width={250}><Icon name="more" size={15} /></MenuButton>}
          </div>
        ))}</div>}
      </div>
      {ban && <BanModal m={ban} onClose={() => setBan(null)} onBan={(days, reason) => { setBan(null); put(ban, { status: 'banned', ban_days: days, ban_reason: reason }, 'Banned') }} />}
      {invite && <InviteModal forum={slug} owner={doc.can.own} onClose={() => setInvite(false)} onDone={load} />}
      {titled && <AskText title={`A title for ${titled.name}`} label="Shown next to their name" placeholder="Treasurer" initial={titled.title} onClose={() => setTitled(null)} onDone={v => { const m = titled; setTitled(null); put(m, { title: v }, 'Saved') }} />}
    </div>
  )
}

function BanModal({ m, onClose, onBan }: { m: Member; onClose: () => void; onBan: (days: number, reason: string) => void }) {
  const [days, setDays] = useState(7)
  const [reason, setReason] = useState('')
  return (
    <Modal title={`Ban ${m.name}`} onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn danger" onClick={() => onBan(days, reason)}>Ban</button></>}>
      <div className="stack">
        <Field label="For" group><Segmented size="sm" value={days} onChange={setDays} options={[{ value: 1, label: 'A day' }, { value: 7, label: 'A week' }, { value: 30, label: 'A month' }, { value: 0, label: 'For good' }]} /></Field>
        <Field label="Why" help="They see this."><input value={reason} onChange={e => setReason(e.target.value)} maxLength={300} /></Field>
        <p className="muted small">They can still read a forum that is open to everyone here, but not post. Their posts stay.</p>
      </div>
    </Modal>
  )
}

function Invites({ doc }: { doc: ForumDoc }) {
  const { toast } = useToast()
  const [list, setList] = useState<{ invite: Invite; url: string }[] | null>(null)
  const [make, setMake] = useState(false)
  const load = useCallback(() => api.get<{ invites: { invite: Invite; url: string }[] }>(`${V1}/forums/${doc.forum.slug}/invites`).then(r => setList(r.invites)).catch(() => setList([])), [doc.forum.slug])
  useEffect(() => { load() }, [load])
  const stop = async (i: Invite) => { try { await api.del(`${V1}/invites/${i.id}`); load(); toast('Stopped') } catch (e) { toast(errMsg(e), 'err') } }
  const state = (i: Invite) => i.revoked ? 'stopped' : i.expires_at && i.expires_at < new Date().toISOString() ? 'ran out' : i.max_uses && i.uses >= i.max_uses ? 'used' : 'works'
  return (
    <div className="stack fs-body">
      <div className="row"><p className="muted small grow">Invite links let people join — even people with no account on this box yet.</p><button type="button" className="btn primary" onClick={() => setMake(true)}><Icon name="plus" size={15} />New invite link</button></div>
      <div className="card flush-card">{!list ? <Loading /> : list.length === 0 ? <Empty title="No invite links yet" icon="mail" /> : <div className="rows">{list.map(({ invite: i, url }) => (
        <div key={i.id} className={`row ${state(i) !== 'works' ? 'dim' : ''}`}>
          <span className="grow"><b>{i.email || i.note || 'Anyone with the link'}</b><span className="muted small">{i.uses}{i.max_uses ? ` of ${i.max_uses}` : ''} used · {i.expires_at ? `until ${fmtDateTime(i.expires_at)}` : 'no end'} · {i.role} · {state(i)}</span></span>
          {state(i) === 'works' && <><button type="button" className="btn sm" onClick={() => { navigator.clipboard.writeText(url); toast('Link copied') }}><Icon name="copy" size={14} />Copy</button><button type="button" className="btn ghost sm" onClick={() => stop(i)}>Stop</button></>}
        </div>
      ))}</div>}</div>
      {make && <InviteModal forum={doc.forum.slug} owner={doc.can.own} onClose={() => setMake(false)} onDone={load} />}
    </div>
  )
}

function Moderation({ doc, save }: { doc: ForumDoc; save: (p: object, ok?: string) => Promise<void> }) {
  const f = doc.forum
  const [blocked, setBlocked] = useState(f.blocked_words.join('\n'))
  const [held, setHeld] = useState(f.held_words.join('\n'))
  const [reacts, setReacts] = useState((f.reactions.length ? f.reactions : doc.reactions).join(' '))
  return (
    <div className="stack loose fs-body">
      <section className="card pad-card">
        <SettingRow title="Hold new members' first posts" help="Their first posts wait in Review until a moderator approves them. Keeps spam out of open forums.">
          <Segmented size="sm" value={f.approve_first} onChange={v => save({ approve_first: v })} options={[{ value: 0, label: 'Off' }, { value: 1, label: 'First' }, { value: 3, label: 'First 3' }, { value: 10, label: 'First 10' }]} /></SettingRow>
        <SettingRow title="Slow mode" help="How long each person waits between two posts anywhere in the forum.">
          <Segmented size="sm" value={f.slow_mode} onChange={v => save({ slow_mode: v })} options={[{ value: 0, label: 'Off' }, { value: 30, label: '30 s' }, { value: 300, label: '5 min' }, { value: 3600, label: '1 h' }]} /></SettingRow>
        <SettingRow title="Editing own posts" help="How long after posting members can still change what they wrote. Moderators can always.">
          <Segmented size="sm" value={f.edit_minutes} onChange={v => save({ edit_minutes: v })} options={[{ value: 0, label: 'Always' }, { value: 60, label: 'An hour' }, { value: 1440, label: 'A day' }, { value: 10080, label: 'A week' }]} /></SettingRow>
      </section>
      <section className="card pad-card stack">
        <Field label="Words that are not allowed" help="One per line. A post with one of them is refused, with the word named."><textarea value={blocked} onChange={e => setBlocked(e.target.value)} rows={4} /></Field>
        <Field label="Words to look at first" help="One per line. A post with one of them waits in Review."><textarea value={held} onChange={e => setHeld(e.target.value)} rows={4} /></Field>
        <Field label="Reactions" help="Up to eight emoji, separated by spaces."><input value={reacts} onChange={e => setReacts(e.target.value)} /></Field>
        <div className="row"><span className="grow" /><button type="button" className="btn primary" onClick={() => save({ blocked_words: blocked.split('\n'), held_words: held.split('\n'), reactions: reacts.split(/\s+/).filter(Boolean) })}>Save</button></div>
      </section>
    </div>
  )
}

function Log({ slug }: { slug: string }) {
  const [list, setList] = useState<LogEntry[] | null>(null)
  useEffect(() => { api.get<LogEntry[]>(`${V1}/forums/${slug}/log`).then(setList).catch(() => setList([])) }, [slug])
  return (
    <div className="card flush-card fs-body">{!list ? <Loading /> : list.length === 0 ? <Empty title="Nothing yet" icon="history" /> : <div className="rows">{list.map(e => (
      <div key={e.id} className="row"><span className="grow"><span><b>{e.actor_name || 'Someone'}</b> {e.action}{e.detail ? <> — <span className="muted">{e.detail}</span></> : null}</span>
        {e.target_kind === 'topic' && e.target_id > 0 && <Link to={`/t/${e.target_id}`} className="small">open the topic</Link>}</span><span className="muted small">{fmtDateTime(e.created_at)}</span></div>
    ))}</div>}</div>
  )
}

function Import({ slug }: { slug: string }) {
  const { toast } = useToast()
  const [url, setUrl] = useState('')
  const [limit, setLimit] = useState(50)
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState('')
  const go = async () => {
    setBusy(true); setRes('')
    try { const r = await api.post<{ text: string }>(`${V1}/forums/${slug}/import/discourse`, { url, limit }); setRes(r.text); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose fs-body">
      <section className="card pad-card stack">
        <h3 className="card-h">From a Discourse forum</h3>
        <p className="muted small">Many forums run on Discourse. Paste the address of one you can read without signing in: its categories and newest topics, with every post, come here. Authors keep their names; running it again only adds what is new.</p>
        <Field label="The forum's address"><input value={url} onChange={e => setUrl(e.target.value)} placeholder="https://forum.example.com" type="url" /></Field>
        <Field label="How many topics" group><Segmented size="sm" value={limit} onChange={setLimit} options={[{ value: 20, label: '20' }, { value: 50, label: '50' }, { value: 150, label: '150' }, { value: 300, label: '300' }]} /></Field>
        <div className="row"><span className="grow muted small">{res}</span><button type="button" className="btn primary" disabled={!/^https?:\/\/.+/.test(url) || busy} onClick={go}>{busy ? <><Spinner size={14} /> Bringing them over…</> : 'Bring them over'}</button></div>
      </section>
      <section className="card pad-card"><p className="muted small">To bring back a Pocket Forum export, use Settings → Backup & data.</p></section>
    </div>
  )
}
