// Writing a post: Markdown with a toolbar, @mentions, pictures and files
// (paste or drop them), things from the other Pocket apps, AI help, a
// preview, and a draft kept as you type.
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useAppState } from '../lib/data'
import { uploadFile } from '../lib/upload'
import { Icon } from './icons'
import { Markdown } from './Markdown'
import { InsertModal, type InsertKind } from './Insert'
import { Face } from './forum'
import { type MenuItem, MenuButton, Spinner, errMsg, useIsMobile, useToast } from './ui'

interface Person { user_id: number; name: string; handle: string; color: string; avatar_id: number }

export interface ComposerHandle { insert: (text: string) => void; focus: () => void }

export function Composer({ value, onChange, forum, draftKey, draftExtra, onSubmit, submitLabel = 'Post', busy, placeholder = 'Write something…', topicId, replyTo, autoFocus,
  onCancel, footerLeft, minHeight = 140, handle, pub, noButton }: {
  value: string; onChange: (v: string) => void; forum: string; draftKey?: string; draftExtra?: Record<string, unknown>; onSubmit?: () => void; submitLabel?: string; busy?: boolean
  placeholder?: string; topicId?: number; replyTo?: number; autoFocus?: boolean; onCancel?: () => void; footerLeft?: React.ReactNode; minHeight?: number
  handle?: React.MutableRefObject<ComposerHandle | null>; pub?: boolean; noButton?: boolean
}) {
  const { settings } = useApp()
  const { toast } = useToast()
  const appState = useAppState()
  const mobile = useIsMobile()
  const ta = useRef<HTMLTextAreaElement>(null)
  const fileIn = useRef<HTMLInputElement>(null)
  const [preview, setPreview] = useState(false)
  const [uploading, setUploading] = useState(0)
  const [aiBusy, setAiBusy] = useState(false)
  const [insert, setInsert] = useState<InsertKind | null>(null)
  const [saved, setSaved] = useState<'' | 'saving' | 'saved'>('')
  const [people, setPeople] = useState<Person[]>([])
  const [mention, setMention] = useState<{ start: number; q: string } | null>(null)
  const [hi, setHi] = useState(0)
  const side = !mobile && !!settings?.preview && !pub

  // the draft: saved a second after the last key
  const first = useRef(true)
  useEffect(() => {
    if (!draftKey || pub) return
    if (first.current) { first.current = false; return }
    setSaved('saving')
    const t = window.setTimeout(() => {
      const body = { body: value, ...draftExtra }
      ;(value.trim() || Object.values(draftExtra || {}).some(Boolean) ? api.put(`${V1}/drafts/${draftKey}`, body) : api.del(`${V1}/drafts/${draftKey}`))
        .then(() => setSaved('saved')).catch(() => setSaved(''))
    }, 900)
    return () => window.clearTimeout(t)
  }, [value, draftKey, JSON.stringify(draftExtra)]) // eslint-disable-line react-hooks/exhaustive-deps

  const setText = useCallback((next: string, caret?: number) => {
    onChange(next)
    if (caret !== undefined) requestAnimationFrame(() => { const el = ta.current; if (el) { el.focus(); el.setSelectionRange(caret, caret) } })
  }, [onChange])

  const insertAt = useCallback((text: string) => {
    const el = ta.current
    const s = el ? el.selectionStart : value.length
    const e = el ? el.selectionEnd : value.length
    const before = value.slice(0, s)
    const pad = before && !before.endsWith('\n') && /^(\n|!\[|\[quote|>|```|[-*] |\d+\.)/.test(text) ? '\n\n' : ''
    const next = before + pad + text + value.slice(e)
    setText(next, s + pad.length + text.length)
  }, [value, setText])

  useEffect(() => {
    if (handle) handle.current = { insert: insertAt, focus: () => ta.current?.focus() }
  }, [handle, insertAt])

  /** wrap the selection in before/after (or put a placeholder in) */
  const wrap = (before: string, after = before, ph = 'text') => {
    const el = ta.current
    if (!el) return
    const s = el.selectionStart, e = el.selectionEnd
    const sel = value.slice(s, e) || ph
    const next = value.slice(0, s) + before + sel + after + value.slice(e)
    onChange(next)
    requestAnimationFrame(() => { el.focus(); el.setSelectionRange(s + before.length, s + before.length + sel.length) })
  }
  /** prefix every selected line */
  const linePrefix = (p: string | ((i: number) => string)) => {
    const el = ta.current
    if (!el) return
    let s = el.selectionStart, e = el.selectionEnd
    s = value.lastIndexOf('\n', s - 1) + 1
    const block = value.slice(s, e) || 'text'
    const out = block.split('\n').map((l, i) => (typeof p === 'string' ? p : p(i)) + l).join('\n')
    setText(value.slice(0, s) + out + value.slice(e), s + out.length)
  }

  const upload = async (files: FileList | File[]) => {
    const list = Array.from(files).slice(0, 10)
    if (!list.length) return
    setUploading(n => n + list.length)
    for (const f of list) {
      try {
        const up = await uploadFile(f, forum)
        insertAt(up.markdown + '\n')
      } catch (e) { toast(errMsg(e), 'err') } finally { setUploading(n => n - 1) }
    }
  }

  // @mentions: who matches what is typed after the @
  const checkMention = (text: string, caret: number) => {
    const m = /(^|[\s(])@([a-zA-Z0-9_.-]{0,30})$/.exec(text.slice(0, caret))
    if (m && !pub) setMention({ start: caret - m[2].length, q: m[2] })
    else setMention(null)
  }
  useEffect(() => {
    if (!mention || !forum) { setPeople([]); return }
    const t = window.setTimeout(() => {
      api.get<Person[]>(`${V1}/forums/${forum}/people?q=${encodeURIComponent(mention.q)}`).then(p => { setPeople(p); setHi(0) }).catch(() => setPeople([]))
    }, 120)
    return () => window.clearTimeout(t)
  }, [mention?.q, forum]) // eslint-disable-line react-hooks/exhaustive-deps
  const pickMention = (p: Person) => {
    if (!mention) return
    const end = mention.start + mention.q.length
    const next = value.slice(0, mention.start) + p.handle + ' ' + value.slice(end)
    setMention(null)
    setText(next, mention.start + p.handle.length + 1)
  }

  const onKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (mention && people.length) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => (h + 1) % people.length); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => (h - 1 + people.length) % people.length); return }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pickMention(people[hi]); return }
      if (e.key === 'Escape') { e.preventDefault(); setMention(null); return }
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); if (!busy && value.trim()) onSubmit?.(); return }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') { e.preventDefault(); wrap('**'); return }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'i') { e.preventDefault(); wrap('*'); return }
    if (e.key === 'Escape' && onCancel) { e.preventDefault(); onCancel() }
  }

  const ai = async (how: 'reply' | 'fix' | 'clearer' | 'shorter' | 'friendlier') => {
    setAiBusy(true)
    try {
      const r = how === 'reply'
        ? await api.post<{ text: string }>(`${V1}/ai/reply`, { topic_id: topicId, reply_to: replyTo || 0, draft: value })
        : await api.post<{ text: string }>(`${V1}/ai/improve`, { text: value, how })
      onChange(r.text)
      toast(how === 'reply' ? 'A draft from the AI — read it before you post' : 'Rewritten — check it before you post')
    } catch (e) { toast(errMsg(e), 'err') } finally { setAiBusy(false) }
  }

  const appItem = (kind: InsertKind, label: string, icon: string, slug: string, scope: string): MenuItem => {
    const s = appState(slug, scope)
    return { label, icon, disabled: !s.ok, hint: s.ok ? undefined : 'not connected', onSelect: () => setInsert(kind) }
  }
  const insertItems = (): MenuItem[] => [
    { label: 'Picture or file from this device', icon: 'paperclip', onSelect: () => fileIn.current?.click() },
    'sep',
    appItem('photo', 'Photo from Pocket Photos', 'image', 'photos', 'photos:read'),
    appItem('note', 'Note from Pocket Notes', 'note', 'notes', 'notes:read'),
    appItem('book', 'Book from Pocket Bookshelf', 'book-open', 'bookshelf', 'books:read'),
    appItem('place', 'Place from Pocket Places', 'pin', 'places', 'places:read'),
    'sep',
    { label: 'Poll — use “Add a poll” below the post', icon: 'poll', disabled: true },
  ]
  const aiItems = (): MenuItem[] => [
    ...(topicId ? [{ label: value.trim() ? 'Finish my reply' : 'Draft a reply', icon: 'sparkles', onSelect: () => ai('reply') } as MenuItem, 'sep' as const] : []),
    { label: 'Fix spelling and grammar', icon: 'check', disabled: !value.trim(), onSelect: () => ai('fix') },
    { label: 'Make it clearer', icon: 'text', disabled: !value.trim(), onSelect: () => ai('clearer') },
    { label: 'Make it shorter', icon: 'minus', disabled: !value.trim(), onSelect: () => ai('shorter') },
    { label: 'Make it friendlier', icon: 'smile', disabled: !value.trim(), onSelect: () => ai('friendlier') },
  ]

  const tool = (icon: string, label: string, run: () => void, hint?: string) =>
    <button type="button" className="cmp-tool" aria-label={label} title={hint ? `${label} (${hint})` : label} onMouseDown={e => e.preventDefault()} onClick={run}><Icon name={icon} size={16} /></button>

  const showPreview = preview || side
  return (
    <div className={`composer ${side ? 'side' : ''}`}
      onDragOver={e => { if (e.dataTransfer.types.includes('Files') && !pub) e.preventDefault() }}
      onDrop={e => { if (e.dataTransfer.files.length && !pub) { e.preventDefault(); upload(e.dataTransfer.files) } }}>
      <div className="cmp-bar" role="toolbar" aria-label="Formatting">
        {tool('bold', 'Bold', () => wrap('**'), '⌘B')}
        {tool('italic', 'Italic', () => wrap('*'), '⌘I')}
        {tool('heading', 'Heading', () => linePrefix('## '))}
        {tool('quote', 'Quote', () => linePrefix('> '))}
        {tool('code', 'Code', () => { const el = ta.current; const sel = el ? value.slice(el.selectionStart, el.selectionEnd) : ''; sel.includes('\n') ? wrap('```\n', '\n```', 'code') : wrap('`', '`', 'code') })}
        {tool('link', 'Link', () => wrap('[', '](https://)', 'text'))}
        {tool('list', 'Bulleted list', () => linePrefix('- '))}
        {tool('list-ordered', 'Numbered list', () => linePrefix(i => `${i + 1}. `))}
        {!pub && tool('at', 'Mention someone', () => { insertAt('@'); setTimeout(() => { const el = ta.current; if (el) checkMention(el.value, el.selectionStart) }, 30) })}
        {!pub && <MenuButton items={insertItems} label="Add a picture, a file, or something from your apps" className="cmp-tool" width={290}><Icon name="paperclip" size={16} /></MenuButton>}
        <span className="grow" />
        {uploading > 0 && <span className="muted small cmp-up"><Spinner size={12} /> uploading</span>}
        {!pub && settings?.ai_help !== false && <MenuButton items={aiItems} label="AI help" className="cmp-tool cmp-ai" align="right" width={230}>{aiBusy ? <Spinner size={14} /> : <Icon name="sparkles" size={16} />}</MenuButton>}
        {!side && <button type="button" className={`cmp-tool ${preview ? 'on' : ''}`} onClick={() => setPreview(p => !p)} aria-pressed={preview} title="Preview"><Icon name="eye" size={16} /></button>}
      </div>
      <div className="cmp-body">
        {(!preview || side) && (
          <div className="cmp-write">
            <textarea ref={ta} value={value} placeholder={placeholder} autoFocus={autoFocus} style={{ minHeight }} aria-label="Your post"
              onChange={e => { onChange(e.target.value); checkMention(e.target.value, e.target.selectionStart) }}
              onKeyDown={onKey} onBlur={() => setTimeout(() => setMention(null), 150)}
              onPaste={e => { const files = Array.from(e.clipboardData.files); if (files.length && !pub) { e.preventDefault(); upload(files) } }} />
            {mention && people.length > 0 && (
              <div className="mention-pop" role="listbox" aria-label="People">
                {people.map((p, i) => (
                  <button key={p.user_id} type="button" role="option" aria-selected={i === hi} className={`mention-opt ${i === hi ? 'hl' : ''}`} onMouseDown={e => { e.preventDefault(); pickMention(p) }}>
                    <Face name={p.name} color={p.color} avatarId={p.avatar_id} size={22} /><b>{p.name}</b><span className="muted">@{p.handle}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
        {showPreview && <div className="cmp-preview" aria-label="Preview">{value.trim() ? <Markdown text={value} pub={pub} /> : <p className="muted">The preview shows up here.</p>}</div>}
      </div>
      <div className="cmp-foot">
        {footerLeft}
        <span className="grow" />
        {draftKey && saved && <span className="muted small" aria-live="polite">{saved === 'saving' ? 'Saving draft…' : 'Draft saved'}</span>}
        {onCancel && <button type="button" className="btn" onClick={onCancel}>Cancel</button>}
        {onSubmit && !noButton && <button type="button" className="btn primary" disabled={busy || !value.trim() || uploading > 0} onClick={onSubmit} title="⌘↩">{busy ? <Spinner size={14} /> : <Icon name="send" size={15} />}{submitLabel}</button>}
      </div>
      <input ref={fileIn} type="file" multiple hidden onChange={e => { if (e.target.files) upload(e.target.files); e.target.value = '' }} />
      {insert && <InsertModal kind={insert} forum={forum} onClose={() => setInsert(null)} onInsert={md => { insertAt(md); setInsert(null) }} />}
    </div>
  )
}
