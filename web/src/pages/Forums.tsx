// Forums: the ones you are in, the ones you could join, and making a new one
// from a starting point (community, help, club, family, team, book club).
import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange } from '../lib/data'
import { COLORS, EMOJIS, JOIN, VISIBILITY } from '../lib/forum'
import type { ForumBrief, ForumDoc, Template, Visibility } from '../lib/types'
import { Icon } from '../components/icons'
import { ForumMark } from '../components/forum'
import { useCrumb } from '../components/Layout'
import { Empty, Field, Loading, Modal, Spinner, errMsg, useToast } from '../components/ui'

export default function Forums() {
  useCrumb(undefined)
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [data, setData] = useState<{ forums: ForumBrief[]; discover: ForumBrief[]; templates: Template[] } | null>(null)
  const load = () => api.get<typeof data>(`${V1}/forums`).then(setData).catch(() => {})
  useEffect(() => { load() }, [])
  const creating = sp.get('new') === '1'
  const join = async (f: ForumBrief) => {
    try { const d = await api.post<ForumDoc>(`${V1}/forums/${f.slug}/join`, {}); announceChange(); if (d.can.join === 'pending') { toast('Asked — a moderator will let you in'); load() } else navigate(`/f/${f.slug}`) } catch (e) { toast(errMsg(e), 'err') }
  }
  if (!data) return <Loading />
  return (
    <div className="page">
      <div className="page-head"><div className="grow"><h1 className="page-title">Forums</h1><p className="muted small">Each forum has its own people, categories and rules.</p></div>
        <button type="button" className="btn primary" onClick={() => setSp({ new: '1' })}><Icon name="plus" size={15} />New forum</button></div>
      {data.forums.length === 0 && data.discover.length === 0 ? <div className="card"><Empty title="No forums yet" icon="forum"><button type="button" className="btn primary" onClick={() => setSp({ new: '1' })}>Make the first one</button></Empty></div> : <>
        {data.forums.length > 0 && <section className="stack"><h2 className="sec-h">Yours</h2>
          <div className="forum-grid">{data.forums.map(f => (
            <Link key={f.id} to={`/f/${f.slug}`} className={`forum-card ${f.archived ? 'dim' : ''}`} style={{ ['--f' as string]: f.color || 'var(--accent)' }}>
              <div className="forum-card-head"><ForumMark f={f} size={40} /><span className="grow"><b>{f.name}</b><span className="muted small">{VISIBILITY[f.visibility].label}{f.role !== 'member' && f.role ? ` · ${f.role === 'owner' ? 'yours' : 'you moderate'}` : ''}{f.status === 'pending' ? ' · asked to join' : ''}</span></span></div>
              {f.about && <p className="muted small">{f.about}</p>}
              <div className="forum-card-foot small"><span><Icon name="users" size={12} /> {f.members}</span><span><Icon name="message" size={12} /> {f.topics}</span>{f.unread > 0 && <span className="t-unread">{f.unread} unread</span>}{f.new > 0 && <span className="t-new">{f.new} new</span>}{(f.pending || 0) > 0 && <span className="t-unread">{f.pending} asking</span>}</div>
            </Link>
          ))}</div></section>}
        {data.discover.length > 0 && <section className="stack"><h2 className="sec-h">You could join</h2>
          <div className="forum-grid">{data.discover.map(f => (
            <div key={f.id} className="forum-card" style={{ ['--f' as string]: f.color || 'var(--accent)' }}>
              <div className="forum-card-head"><ForumMark f={f} size={40} /><span className="grow"><Link to={`/f/${f.slug}`}><b>{f.name}</b></Link><span className="muted small">{JOIN[f.join_policy].label}</span></span></div>
              {f.about && <p className="muted small">{f.about}</p>}
              <div className="forum-card-foot small"><span><Icon name="users" size={12} /> {f.members}</span><span><Icon name="message" size={12} /> {f.topics}</span><span className="grow" />
                <Link to={`/f/${f.slug}`} className="btn sm">Look</Link>
                {f.join_policy !== 'invite' && <button type="button" className="btn primary sm" onClick={() => join(f)}>{f.join_policy === 'open' ? 'Join' : 'Ask to join'}</button>}</div>
            </div>
          ))}</div></section>}
      </>}
      {creating && <NewForum templates={data.templates} onClose={() => setSp({})} onMade={slug => { announceChange(); navigate(`/f/${slug}`) }} />}
    </div>
  )
}

function NewForum({ templates, onClose, onMade }: { templates: Template[]; onClose: () => void; onMade: (slug: string) => void }) {
  const { toast } = useToast()
  const [tpl, setTpl] = useState<Template | null>(null)
  const [name, setName] = useState('')
  const [about, setAbout] = useState('')
  const [emoji, setEmoji] = useState('💬')
  const [color, setColor] = useState(COLORS[0])
  const [vis, setVis] = useState<Visibility>('private')
  const [busy, setBusy] = useState(false)
  const pick = (t: Template) => { setTpl(t); setEmoji(t.emoji); setVis(t.visibility) }
  const make = async () => {
    setBusy(true)
    try {
      const d = await api.post<ForumDoc>(`${V1}/forums`, { name, about, emoji, color, visibility: vis, join_policy: vis === 'private' ? 'invite' : tpl?.join_policy || 'open', template: tpl?.id })
      toast(`${d.forum.name} is ready`)
      onMade(d.forum.slug)
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={tpl ? `New forum: ${tpl.name}` : 'What is the forum for?'} onClose={onClose} size="lg" footer={tpl ? <>
      <button type="button" className="btn" onClick={() => setTpl(null)}>Back</button><span className="grow" />
      <button type="button" className="btn primary" disabled={name.trim().length < 2 || busy} onClick={make}>{busy && <Spinner size={14} />}Make the forum</button></> : undefined}>
      {!tpl ? (
        <div className="tpl-grid">{templates.map(t => (
          <button key={t.id} type="button" className="tpl-card" onClick={() => pick(t)}>
            <span className="tpl-emoji" aria-hidden="true">{t.emoji}</span><b>{t.name}</b><span className="muted small">{t.line}</span>
            <span className="chips">{t.categories.slice(0, 5).map(c => <span key={c.name} className="chip-s">{c.emoji} {c.name}</span>)}</span>
          </button>
        ))}</div>
      ) : (
        <div className="stack">
          <div className="row"><span className="forum-mark big" style={{ background: `color-mix(in srgb, ${color} 16%, transparent)` }}>{emoji}</span>
            <Field label="Name" className="grow"><input value={name} onChange={e => setName(e.target.value)} placeholder={tpl.id === 'family' ? 'The Kovač family' : tpl.id === 'club' ? 'Old Boys FC' : 'Our forum'} autoFocus maxLength={80} /></Field></div>
          <Field label="One line about it"><input value={about} onChange={e => setAbout(e.target.value)} maxLength={300} placeholder="What people find here" /></Field>
          <Field label="Picture" group><div className="emoji-pick">{EMOJIS.map(e => <button key={e} type="button" className={emoji === e ? 'on' : ''} onClick={() => setEmoji(e)}>{e}</button>)}</div></Field>
          <Field label="Colour" group><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${color === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setColor(c)} aria-label={c} />)}</div></Field>
          <Field label="Who can see it" group>
            <div className="stack tight">{(Object.keys(VISIBILITY) as Visibility[]).map(v => (
              <label key={v} className={`radio-row ${vis === v ? 'on' : ''}`}><input type="radio" name="vis" checked={vis === v} onChange={() => setVis(v)} /><Icon name={VISIBILITY[v].icon} size={16} /><span><b>{VISIBILITY[v].label}</b><p className="muted small">{VISIBILITY[v].line}{v === 'public' ? ' (on this box’s network)' : ''}</p></span></label>
            ))}</div>
          </Field>
          <p className="muted small">It starts with the categories {tpl.categories.map(c => c.name).join(', ')} — change them any time in Forum settings.</p>
        </div>
      )}
    </Modal>
  )
}
