// Putting something from another Pocket app in a post: a photo, a note,
// a book, a place.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { Icon } from './icons'
import { Empty, Loading, Modal, Spinner, errMsg, useDebounce, useToast } from './ui'

export type InsertKind = 'photo' | 'note' | 'book' | 'place'

const TITLES: Record<InsertKind, string> = { photo: 'A photo from Pocket Photos', note: 'A note from Pocket Notes', book: 'A book from Pocket Bookshelf', place: 'A place from Pocket Places' }

interface Row { id: string; title: string; sub?: string; thumb?: string; url?: string }

export function InsertModal({ kind, forum, onClose, onInsert }: { kind: InsertKind; forum: string; onClose: () => void; onInsert: (md: string) => void }) {
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 300)
  const [rows, setRows] = useState<Row[] | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState('')
  useEffect(() => {
    setErr('')
    const enc = encodeURIComponent(dq)
    const load = async (): Promise<Row[]> => {
      switch (kind) {
        case 'photo': {
          const r = await api.get<{ photos: { id: string; name: string; date: string; thumb_url: string }[] }>(`${V1}/apps/photos?q=${enc}`)
          return r.photos.map(p => ({ id: p.id, title: p.name, sub: p.date, thumb: p.thumb_url }))
        }
        case 'note': {
          if (!dq.trim()) return []
          const r = await api.get<{ notes: { id: string; title: string; folder: string; snippet: string }[] }>(`${V1}/apps/notes?q=${enc}`)
          return r.notes.map(n => ({ id: n.id, title: n.title, sub: [n.folder, n.snippet].filter(Boolean).join(' · ') }))
        }
        case 'book': {
          const r = await api.get<{ books: { id: string; title: string; author: string; url: string }[] }>(`${V1}/apps/books?q=${enc}`)
          return r.books.map(b => ({ id: b.id, title: b.title, sub: b.author, url: b.url }))
        }
        case 'place': {
          const r = await api.get<{ places: { id: string; name: string; address: string; url: string }[] }>(`${V1}/apps/places?q=${enc}`)
          return r.places.map(p => ({ id: p.id, title: p.name, sub: p.address, url: p.url }))
        }
      }
    }
    load().then(setRows).catch(e => { setErr(errMsg(e)); setRows([]) })
  }, [kind, dq])

  const pick = async (r: Row) => {
    setBusy(r.id)
    try {
      switch (kind) {
        case 'photo': {
          const up = await api.post<{ markdown: string }>(`${V1}/apps/photos/${r.id}/copy?forum=${encodeURIComponent(forum)}`)
          onInsert(up.markdown + '\n')
          break
        }
        case 'note': {
          const n = await api.get<{ title: string; content: string; url: string }>(`${V1}/apps/notes/${r.id}`)
          const body = n.content.split('\n').filter((l, i) => !(i === 0 && l.trim() === '# ' + n.title)).join('\n').trim()
          onInsert(`📝 **${n.url ? `[${n.title}](${n.url})` : n.title}**\n\n${body.split('\n').map(l => '> ' + l).join('\n')}\n`)
          break
        }
        case 'book':
          onInsert(`📚 ${r.url ? `[${r.title}](${r.url})` : `**${r.title}**`}${r.sub ? ` — ${r.sub}` : ''}\n`)
          break
        case 'place':
          onInsert(`📍 ${r.url ? `[${r.title}](${r.url})` : `**${r.title}**`}${r.sub ? ` — ${r.sub}` : ''}\n`)
          break
      }
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy('') }
  }

  return (
    <Modal title={TITLES[kind]} onClose={onClose} size={kind === 'photo' ? 'lg' : ''}>
      <div className="stack">
        <div className="add-search"><Icon name="search" size={15} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder={kind === 'photo' ? 'Search your photos (a beach, a cat, 2019)…' : kind === 'note' ? 'Search your notes…' : 'Search…'} /></div>
        {err && <p className="error-text">{err}</p>}
        {rows === null ? <Loading /> : rows.length === 0 && !err ? <Empty title={kind === 'note' && !dq.trim() ? 'Type to find a note' : 'Nothing found'} icon="search" /> : kind === 'photo' ? (
          <div className="ins-grid">
            {rows.map(r => <button key={r.id} type="button" className="ins-photo" onClick={() => pick(r)} disabled={!!busy} title={r.title}>
              <img src={r.thumb} alt={r.title} loading="lazy" />{busy === r.id && <span className="ins-busy"><Spinner /></span>}
            </button>)}
          </div>
        ) : (
          <div className="rows ins-rows">
            {rows.map(r => <button key={r.id} type="button" className="row" onClick={() => pick(r)} disabled={!!busy}>
              <span className="grow"><b className="ellipsis">{r.title}</b>{r.sub && <span className="muted small ellipsis">{r.sub}</span>}</span>
              {busy === r.id ? <Spinner /> : <Icon name="plus" size={16} />}
            </button>)}
          </div>
        )}
      </div>
    </Modal>
  )
}
