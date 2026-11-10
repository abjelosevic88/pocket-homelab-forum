// Inviting people: an invite link (for anyone, or for one email), shown as a
// link and a QR code, sent from your own mail through Pocket Email with
// people picked from Pocket Contacts — or adding someone who already has an
// account on this box.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useAppState } from '../lib/data'
import type { Invite } from '../lib/types'
import { Icon } from './icons'
import { QRCode } from './QRCode'
import { CopyField, Field, Modal, Segmented, Spinner, errMsg, useDebounce, useToast } from './ui'

interface Contact { id: string; name: string; emails: string[] }

export function InviteModal({ forum, owner, onClose, onDone }: { forum: string; owner: boolean; onClose: () => void; onDone?: () => void }) {
  const { toast } = useToast()
  const appState = useAppState()
  const [how, setHow] = useState<'link' | 'account'>('link')
  const [note, setNote] = useState('')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'member' | 'moderator'>('member')
  const [days, setDays] = useState(14)
  const [maxUses, setMaxUses] = useState(0)
  const [made, setMade] = useState<{ invite: Invite; url: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [to, setTo] = useState<{ name: string; addr: string }[]>([])
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [contacts, setContacts] = useState<Contact[]>([])
  const mail = appState('email', 'mail:send')
  const book = appState('contacts', 'contacts:read')
  useEffect(() => {
    if (!made || !book.ok || dq.trim().length < 2) { setContacts([]); return }
    api.get<{ contacts: Contact[] }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq)}`).then(r => setContacts(r.contacts)).catch(() => setContacts([]))
  }, [dq, made, book.ok])

  const make = async () => {
    setBusy(true)
    try {
      if (how === 'account') {
        await api.post(`${V1}/forums/${forum}/members`, { email, role })
        toast(`${email} is in`)
        onDone?.()
        onClose()
        return
      }
      const r = await api.post<{ invite: Invite; url: string }>(`${V1}/forums/${forum}/invites`, { note, email, role, days, max_uses: maxUses })
      setMade(r)
      if (email) setTo([{ name: '', addr: email }])
      onDone?.()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const send = async () => {
    if (!made) return
    setBusy(true)
    try {
      const r = await api.post<{ text: string }>(`${V1}/apps/email`, { invite_id: made.invite.id, to, text: note })
      toast(r.text)
      onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const addAddr = (addr: string, name = '') => { addr = addr.trim(); if (addr.includes('@') && !to.some(t => t.addr === addr)) setTo([...to, { name, addr }]); setQ('') }

  return (
    <Modal title="Invite people" onClose={onClose} footer={made ? <>
      <button type="button" className="btn" onClick={onClose}>Done</button>
      <button type="button" className="btn primary" disabled={!mail.ok || !to.length || busy} onClick={send} title={mail.ok ? '' : 'Connect Pocket Email (Settings → Connected apps) to send it from here'}>{busy ? <Spinner size={14} /> : <Icon name="mail" size={15} />}Send by email</button>
    </> : <>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={busy || (how === 'account' && !email.includes('@'))} onClick={make}>{busy ? <Spinner size={14} /> : null}{how === 'account' ? 'Add them' : 'Make the link'}</button>
    </>}>
      {!made ? (
        <div className="stack">
          <Segmented value={how} onChange={setHow} options={[{ value: 'link', label: 'An invite link' }, { value: 'account', label: 'Someone with an account here' }]} />
          {how === 'link' ? <>
            <Field label="Who is it for?" help="Leave it empty for a link anyone can use. With an email, only that person can use it, once."><input value={email} onChange={e => setEmail(e.target.value)} placeholder="Anyone with the link" type="email" /></Field>
            <Field label="A note for yourself or the email" help="For example “for the parents of the U10 team”."><input value={note} onChange={e => setNote(e.target.value)} maxLength={200} /></Field>
            <div className="grid two">
              <Field label="Works for" group><Segmented size="sm" value={days} onChange={setDays} options={[{ value: 1, label: 'A day' }, { value: 14, label: '2 weeks' }, { value: 90, label: '3 months' }, { value: 0, label: 'Always' }]} /></Field>
              {!email && <Field label="How many people" group><Segmented size="sm" value={maxUses} onChange={setMaxUses} options={[{ value: 1, label: 'One' }, { value: 10, label: 'Ten' }, { value: 0, label: 'Any' }]} /></Field>}
            </div>
          </> : <Field label="Their email on this box" help="They join straight away and get a notification. For someone without an account, make an invite link instead."><input value={email} onChange={e => setEmail(e.target.value)} type="email" autoFocus /></Field>}
          {owner && <Field label="As" group><Segmented size="sm" value={role} onChange={setRole} options={[{ value: 'member', label: 'Member' }, { value: 'moderator', label: 'Moderator' }]} /></Field>}
        </div>
      ) : (
        <div className="stack">
          <p className="muted">Anyone who opens this link {made.invite.email ? `with ${made.invite.email}` : ''} can join{made.invite.accounts ? ' — and make an account on this box if they have none' : ''}.</p>
          <CopyField value={made.url} label="Invite link" />
          <div className="invite-qr"><QRCode value={made.url} size={168} label="Invite link as a QR code" /><span className="muted small">Show this to someone in the room: their phone camera opens the link.</span></div>
          <Field label="Send it by email" group help={mail.ok ? 'It goes from your own mail account through Pocket Email.' : 'Connect Pocket Email in Settings → Connected apps to send it from here.'}>
            <div className="to-list">
              {to.map(t => <span key={t.addr} className="chip on">{t.name || t.addr}<button type="button" className="chip-x" aria-label={`Remove ${t.addr}`} onClick={() => setTo(to.filter(x => x.addr !== t.addr))}><Icon name="x" size={12} /></button></span>)}
              <input value={q} onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); addAddr(q) } }} placeholder={book.ok ? 'Name from your contacts, or an email' : 'Email address'} disabled={!mail.ok} />
            </div>
            {contacts.length > 0 && <div className="rows mini-rows">{contacts.slice(0, 6).flatMap(c => c.emails.slice(0, 2).map(e => <button key={c.id + e} type="button" className="row" onClick={() => addAddr(e, c.name)}><span className="grow"><b>{c.name}</b><span className="muted small">{e}</span></span><Icon name="plus" size={15} /></button>))}</div>}
          </Field>
        </div>
      )}
    </Modal>
  )
}
