// A safe Markdown reader for posts: paragraphs, headings, lists, tables,
// code blocks and `inline code`, > quotes and [quote=handle #N] quoted posts,
// bold/italic/strike, links (and bare addresses), pictures, @mentions,
// ||spoilers||. No HTML gets in.
import React from 'react'
import { Link } from 'react-router-dom'

export interface MdOptions {
  /** public pages fetch uploads through the public address */
  pub?: boolean
  /** the topic, so a quote's "#3" can jump there */
  topicId?: number
  /** show pictures; off: a tap shows each */
  images?: boolean
  onQuoteClick?: (n: number) => void
}

const safeURL = (u: string): string => {
  u = u.trim()
  if (/^(https?:|mailto:)/i.test(u)) return u
  if (u.startsWith('/') && !u.startsWith('//')) return u
  return ''
}

function fixUpload(u: string, pub?: boolean) {
  return pub ? u.replace(/^\/api\/v1\/uploads\//, '/api/v1/public/uploads/') : u
}

function Spoiler({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false)
  return <span className={`md-spoiler ${open ? 'open' : ''}`} role="button" tabIndex={0} title={open ? undefined : 'Tap to show'} onClick={() => setOpen(true)} onKeyDown={e => { if (e.key === 'Enter') setOpen(true) }}>{children}</span>
}

function LazyImage({ src, alt, show }: { src: string; alt: string; show: boolean }) {
  const [on, setOn] = React.useState(show)
  if (!on) return <button type="button" className="md-img-hidden" onClick={() => setOn(true)}>🖼 {alt || 'Picture'} — tap to show</button>
  return <a href={src} target="_blank" rel="noreferrer" className="md-img-link"><img src={src} alt={alt} loading="lazy" className="md-img" /></a>
}

const INLINE = /(`[^`\n]+`)|(!\[[^\]]*\]\([^)\s]+\))|(\[[^\]]+\]\([^)\s]+\))|(\|\|[^|]+\|\|)|(\*\*\*[^*]+\*\*\*)|(\*\*[^*]+\*\*)|(~~[^~]+~~)|((?:^|(?<=[\s(]))[*_][^*_\s][^*_]*[*_])|((?:^|(?<=[^\w@/.]))@[a-zA-Z0-9][a-zA-Z0-9_.-]{1,29})|(https?:\/\/[^\s<>()]+[^\s<>().,!?;:'"])/g

function inline(text: string, o: MdOptions, key = ''): React.ReactNode[] {
  const out: React.ReactNode[] = []
  let last = 0
  let i = 0
  let m: RegExpExecArray | null
  const re = new RegExp(INLINE.source, 'g')
  while ((m = re.exec(text))) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const t = m[0]
    const k = key + i++
    if (m[1]) out.push(<code key={k} className="md-code">{t.slice(1, -1)}</code>)
    else if (m[2]) {
      const p = /^!\[([^\]]*)\]\(([^)\s]+)\)$/.exec(t)!
      const src = safeURL(fixUpload(p[2], o.pub))
      out.push(src ? <LazyImage key={k} src={src} alt={p[1]} show={o.images !== false} /> : p[1])
    } else if (m[3]) {
      const p = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(t)!
      const href = safeURL(fixUpload(p[2], o.pub))
      const isFile = /\/uploads\/\d+$/.test(href)
      out.push(href ? (href.startsWith('/') && !href.startsWith('/api/') ? <Link key={k} to={href}>{inline(p[1], o, k + '-')}</Link>
        : <a key={k} href={href} target="_blank" rel="noreferrer" className={isFile ? 'md-file' : undefined}>{isFile && '📎 '}{inline(p[1], o, k + '-')}</a>) : p[1])
    } else if (m[4]) out.push(<Spoiler key={k}>{inline(t.slice(2, -2), o, k + '-')}</Spoiler>)
    else if (m[5]) out.push(<b key={k}><i>{t.slice(3, -3)}</i></b>)
    else if (m[6]) out.push(<b key={k}>{inline(t.slice(2, -2), o, k + '-')}</b>)
    else if (m[7]) out.push(<s key={k}>{t.slice(2, -2)}</s>)
    else if (m[8]) out.push(<i key={k}>{t.slice(1, -1)}</i>)
    else if (m[9]) {
      const h = t.slice(1).replace(/\.$/, '')
      out.push(o.pub ? <span key={k} className="md-mention">@{h}</span> : <Link key={k} to={`/u/${encodeURIComponent(h)}`} className="md-mention">@{h}</Link>)
      if (t.endsWith('.')) out.push('.')
    } else if (m[10]) out.push(<a key={k} href={t} target="_blank" rel="noreferrer">{t.replace(/^https?:\/\//, '').slice(0, 60)}{t.length > 68 ? '…' : ''}</a>)
    last = m.index + t.length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function lines(text: string, o: MdOptions, key: string): React.ReactNode[] {
  const parts = text.split('\n')
  const out: React.ReactNode[] = []
  parts.forEach((p, j) => {
    if (j > 0) out.push(<br key={key + 'br' + j} />)
    out.push(...inline(p, o, key + j + '-'))
  })
  return out
}

const LIST = /^\s*([-*•+]|\d+[.)])\s+/

function blocks(src: string, o: MdOptions, depth = 0): React.ReactNode[] {
  const ls = src.replace(/\r/g, '').split('\n')
  const out: React.ReactNode[] = []
  let i = 0
  let k = 0
  while (i < ls.length) {
    const l = ls[i]
    const t = l.trim()
    if (!t) { i++; continue }
    if (t.startsWith('```')) {
      const lang = t.slice(3).trim()
      const code: string[] = []
      for (i++; i < ls.length && !ls[i].trim().startsWith('```'); i++) code.push(ls[i])
      i++
      out.push(<pre key={k++} className="md-pre" data-lang={lang || undefined}><code>{code.join('\n')}</code></pre>)
      continue
    }
    const q = /^\[quote=([a-zA-Z0-9_.-]+)(?:\s+#(\d+))?\]\s*(.*)$/.exec(t)
    if (q && depth < 4) {
      const inner: string[] = []
      const rest = q[3]
      let depthQ = 1
      if (rest.includes('[/quote]')) { inner.push(rest.split('[/quote]')[0]); i++ }
      else {
        if (rest) inner.push(rest)
        for (i++; i < ls.length; i++) {
          if (/^\[quote=/.test(ls[i].trim())) depthQ++
          if (ls[i].includes('[/quote]')) {
            depthQ--
            if (depthQ === 0) { const before = ls[i].split('[/quote]')[0]; if (before.trim()) inner.push(before); i++; break }
          }
          inner.push(ls[i])
        }
      }
      const n = q[2] ? Number(q[2]) : 0
      out.push(
        <blockquote key={k++} className="md-quote">
          <div className="md-quote-head">
            {o.pub ? <b>@{q[1]}</b> : <Link to={`/u/${encodeURIComponent(q[1])}`}>@{q[1]}</Link>}
            {n > 0 && <>{' wrote in '}{o.onQuoteClick ? <button type="button" className="linkish" onClick={() => o.onQuoteClick!(n)}>#{n}</button> : o.topicId ? <Link to={`/t/${o.topicId}/${n}`}>#{n}</Link> : `#${n}`}</>}
            {!n && ' wrote'}
          </div>
          {blocks(inner.join('\n'), o, depth + 1)}
        </blockquote>)
      continue
    }
    if (t.startsWith('>')) {
      const inner: string[] = []
      for (; i < ls.length && ls[i].trim().startsWith('>'); i++) inner.push(ls[i].trim().replace(/^>\s?/, ''))
      out.push(<blockquote key={k++} className="md-bq">{blocks(inner.join('\n'), o, depth + 1)}</blockquote>)
      continue
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(t)
    if (h) {
      const Tag = (`h${Math.min(6, h[1].length + 2)}`) as 'h3'
      out.push(<Tag key={k++} className="md-h">{inline(h[2], o)}</Tag>)
      i++
      continue
    }
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(t)) { out.push(<hr key={k++} className="md-hr" />); i++; continue }
    if (/^\|.*\|$/.test(t)) {
      const rows: string[][] = []
      while (i < ls.length && /^\|.*\|$/.test(ls[i].trim())) {
        const cells = ls[i].trim().slice(1, -1).split('|').map(c => c.trim())
        if (!cells.every(c => /^:?-{2,}:?$/.test(c))) rows.push(cells)
        i++
      }
      const [head, ...body] = rows
      out.push(<div key={k++} className="md-table-wrap"><table className="md-table"><thead><tr>{head.map((c, j) => <th key={j}>{inline(c, o)}</th>)}</tr></thead>
        <tbody>{body.map((r, j) => <tr key={j}>{r.map((c, n) => <td key={n}>{inline(c, o)}</td>)}</tr>)}</tbody></table></div>)
      continue
    }
    if (LIST.test(l)) {
      const ordered = /^\s*\d+[.)]/.test(l)
      const items: string[] = []
      while (i < ls.length && (LIST.test(ls[i]) || (/^\s{2,}\S/.test(ls[i]) && items.length))) {
        if (LIST.test(ls[i])) items.push(ls[i].replace(LIST, ''))
        else items[items.length - 1] += '\n' + ls[i].trim()
        i++
      }
      const L = ordered ? 'ol' : 'ul'
      out.push(<L key={k++} className="md-list">{items.map((it, j) => {
        const task = /^\[( |x|X)\]\s+([\s\S]*)$/.exec(it)
        if (task) return <li key={j} className="md-task"><span className={`md-box ${task[1] !== ' ' ? 'on' : ''}`} aria-hidden="true">{task[1] !== ' ' ? '✓' : ''}</span>{lines(task[2], o, `${k}-${j}-`)}</li>
        return <li key={j}>{lines(it, o, `${k}-${j}-`)}</li>
      })}</L>)
      continue
    }
    const para: string[] = []
    while (i < ls.length && ls[i].trim() && !/^(```|#{1,6}\s|>|\[quote=|\||(-{3,}|\*{3,})$)/.test(ls[i].trim()) && !LIST.test(ls[i])) { para.push(ls[i]); i++ }
    if (para.length === 0) { out.push(<p key={k++}>{inline(ls[i], o)}</p>); i++; continue }
    out.push(<p key={k++}>{lines(para.join('\n'), o, `${k}-`)}</p>)
  }
  return out
}

export function Markdown({ text, className = '', ...o }: { text: string; className?: string } & MdOptions) {
  return <div className={`md ${className}`}>{blocks(text || '', o)}</div>
}

/** A short plain-text version of a post, for previews and lists. */
export function plain(md: string, n = 180): string {
  const t = (md || '')
    .replace(/\[quote=[^\]]*\][\s\S]*?\[\/quote\]/g, ' ')
    .replace(/```[\s\S]*?```/g, ' [code] ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' [picture] ')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[`*_~#>|]+/g, '')
    .replace(/\s+/g, ' ').trim()
  return t.length > n ? t.slice(0, n - 1).trimEnd() + '…' : t
}

/** Search snippets come with ⟦…⟧ around the words found. */
export function Snippet({ text }: { text: string }) {
  const parts = plain(text.replace(/⟦/g, '\u0001').replace(/⟧/g, '\u0002'), 400).split(/(\u0001[^\u0002]*\u0002)/)
  return <>{parts.map((p, i) => p.startsWith('\u0001') ? <mark key={i}>{p.slice(1, -1)}</mark> : p)}</>
}
