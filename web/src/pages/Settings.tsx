import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { APP_INFO, COLORS, STATE_WORDS } from '../lib/forum'
import type { Profile as ProfileT } from '../lib/types'
import { Face } from '../components/forum'
import { uploadFile } from '../lib/upload'
import { upload } from '../lib/api'

type TabId = 'general' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'What the home page shows, following what you reply to, writing, pictures, dates and the app title.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'What reaches your phone, and the summary email of what you missed.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Chat, Email, Contacts, Calendar, Tasks, Projects, Notes, Files, Photos, Learn, Blog, Bookshelf, Places and Translate.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, your @handle, the line under your name, your picture, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Forums decide for themselves who is in them.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export everything, bring an export back, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Reading" icon="eye">
        <SettingRow title="The home page opens on" help="You can always switch with the tabs."><Segmented size="sm" value={f.home_tab} onChange={v => setF({ ...f, home_tab: v })} options={[{ value: 'latest', label: 'Latest' }, { value: 'unread', label: 'Unread' }, { value: 'new', label: 'New' }, { value: 'top', label: 'Top' }]} /></SettingRow>
        <SettingRow title="Lists" help="Compact shows more topics at once, without the first lines."><Segmented size="sm" value={f.density} onChange={v => setF({ ...f, density: v })} options={[{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }]} /></SettingRow>
        <SettingRow title="Pictures in posts" help="Off: each picture waits for a tap (saves data on a phone)."><Switch checked={f.load_images} onChange={v => setF({ ...f, load_images: v })} label="Pictures" /></SettingRow>
      </Section>
      <Section title="Writing and following" icon="edit">
        <SettingRow title="Follow what you take part in" help="Topics you start are watched (every new post); topics you reply to are tracked (new posts counted)."><Switch checked={f.auto_watch} onChange={v => setF({ ...f, auto_watch: v })} label="Follow" /></SettingRow>
        <SettingRow title="Preview beside the text" help="On a wide screen, the post as it will look while you write."><Switch checked={f.preview} onChange={v => setF({ ...f, preview: v })} label="Preview" /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { toast } = useToast()
  const [sending, setSending] = useState(false)
  if (!f) return <Loading />
  const emailOn = f.email_via !== ''
  const testDigest = async () => {
    setSending(true)
    try { const r = await api.post<{ text: string }>(`${V1}/digest/test`); toast(r.text) } catch (e) { toast((e as Error).message, 'err') } finally { setSending(false) }
  }
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON. Nothing is pushed while the forum is open in front of you.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/forum" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to push" icon="bell" sub="Everything also shows under Notifications in the app.">
        <SettingRow title="Someone mentions or quotes you"><Switch checked={f.notify_mentions} onChange={v => setF({ ...f, notify_mentions: v })} label="Mentions" /></SettingRow>
        <SettingRow title="Someone replies to you"><Switch checked={f.notify_replies} onChange={v => setF({ ...f, notify_replies: v })} label="Replies" /></SettingRow>
        <SettingRow title="New posts in topics you watch"><Switch checked={f.notify_watching} onChange={v => setF({ ...f, notify_watching: v })} label="Watching" /></SettingRow>
        <SettingRow title="New topics in forums and categories you watch"><Switch checked={f.notify_new_topics} onChange={v => setF({ ...f, notify_new_topics: v })} label="New topics" /></SettingRow>
        <SettingRow title="Reactions and accepted answers"><Switch checked={f.notify_likes} onChange={v => setF({ ...f, notify_likes: v })} label="Reactions" /></SettingRow>
        <SettingRow title="For moderators: held posts, flags, people asking to join"><Switch checked={f.notify_review} onChange={v => setF({ ...f, notify_review: v })} label="Review" /></SettingRow>
      </Section>
      <Section title="Summary email" icon="mail" sub={emailOn ? 'What you missed, from your own mail account through Pocket Email — only when you were not here since the last one.' : 'Connect Pocket Email (Connected apps) to get a summary of what you missed.'}>
        <SettingRow title="How often"><Segmented size="sm" value={f.digest} onChange={v => setF({ ...f, digest: v })} options={[{ value: 'off', label: 'Never' }, { value: 'daily', label: 'Daily' }, { value: 'weekly', label: 'Weekly' }]} /></SettingRow>
        {f.digest === 'weekly' && <SettingRow title="On"><Select ariaLabel="Day" value={String(f.digest_day)} onChange={v => setF({ ...f, digest_day: Number(v) })} options={['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((d, i) => ({ value: String(i + 1), label: d }))} /></SettingRow>}
        <SettingRow title="At" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
        <SettingRow title="Send one now" help="This week's summary, to see what it looks like."><button type="button" className="btn sm" disabled={!emailOn || sending} onClick={testDigest}><Icon name="mail" size={14} />Send</button></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test push</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'chat', title: 'Pocket Chat — tell the group', sub: 'Share a topic or a post into a chat as a card with its link.', token: 'Pocket Chat → Settings → API tokens', port: '8247' },
  { slug: 'email', title: 'Pocket Email — invitations and summaries', sub: 'Email invitations and topics from your own mail account, and get the summary of what you missed.', token: 'Pocket Email → Settings → API tokens', port: '8174' },
  { slug: 'contacts', title: 'Pocket Contacts — who to invite', sub: 'Pick the people to invite from your address book.', token: 'Pocket Contacts → Settings → API tokens', port: '8172' },
  { slug: 'calendar', title: 'Pocket Calendar — events', sub: 'Put an event topic in your calendar, with the link to it.', token: 'Pocket Calendar → Settings → API tokens', port: '8171' },
  { slug: 'tasks', title: 'Pocket Tasks — follow up', sub: 'Turn a post into a task, to answer it later.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
  { slug: 'projects', title: 'Pocket Projects — issues', sub: 'Turn a bug report or an idea into an issue for your team, with a link back.', token: 'Pocket Projects → Settings → API tokens', port: '8183' },
  { slug: 'notes', title: 'Pocket Notes — keep and quote', sub: 'Save a topic or its summary as a note; quote one of your notes in a post.', token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'files', title: 'Pocket Files — keep a copy', sub: 'Save a whole topic as a file.', token: 'Pocket Files → Settings → API tokens', port: '8187' },
  { slug: 'photos', title: 'Pocket Photos — pictures', sub: 'Put your photos in a post without downloading them first.', token: 'Pocket Photos → Settings → API tokens', port: '8194' },
  { slug: 'learn', title: 'Pocket Learn — remember answers', sub: 'Turn a question and its accepted answer into a flashcard.', token: 'Pocket Learn → Settings → API tokens', port: '8202' },
  { slug: 'blog', title: 'Pocket Blog — publish', sub: 'Start a blog post from a topic, with credit to the people in it.', token: 'Pocket Blog → Settings → API tokens', port: '8246' },
  { slug: 'bookshelf', title: 'Pocket Bookshelf — book clubs', sub: 'Put a book from your shelf in a post.', token: 'Pocket Bookshelf → Settings → API tokens', port: '8190' },
  { slug: 'places', title: 'Pocket Places — where', sub: 'Use one of your saved places in a post or an event.', token: 'Pocket Places → Settings → API tokens', port: '8236' },
  { slug: 'translate', title: 'Pocket Translate — other languages', sub: 'Read a post written in another language in yours (without it, the AI translates).', token: 'Pocket Translate → Settings → API tokens', port: '8251' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [APPS.map(a => f?.[`${a.slug}_mode`]).join()])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Forum only adds things in them when you ask, and everything here works without them. Through <b>Pocket Core</b> Forum acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <Section title="Where things go" icon="layers">
        <SettingRow title="Notes folder" help="Where saved topics go in Pocket Notes."><input value={f.notes_folder} onChange={e => setF({ ...f, notes_folder: e.target.value })} /></SettingRow>
        <SettingRow title="Files folder" help="Where saved topics go in Pocket Files."><input value={f.files_folder} onChange={e => setF({ ...f, files_folder: e.target.value })} /></SettingRow>
        <SettingRow title="Chat" help="The Pocket Chat conversation topics are shared in; empty: a note to yourself."><input value={f.chat_to} onChange={e => setF({ ...f, chat_to: e.target.value })} placeholder="me" /></SettingRow>
        <SettingRow title="Pocket Learn deck" help="Where answers become flashcards (made if it is not there)."><input value={f.learn_deck} onChange={e => setF({ ...f, learn_deck: e.target.value })} /></SettingRow>
        <TeamPick f={f} setF={setF} />
      </Section>
      <Section title="Other Pocket apps" icon="layers" sub="Forum offers your forums to the rest of the box through Pocket Core: Pocket AI can tell you what is new, answer from the forum or post for you when you ask; Pocket Search finds topics; Pocket Day shows what you wrote; Pocket Blog can start a discussion for a post. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>forum:read</dt><dd className="muted">The forums you are in: topics and posts, what is new and unread for you, your notifications and bookmarks, events and open questions.</dd></div>
          <div><dt>forum:write</dt><dd className="muted">Start a topic or reply as you, react and mark read; that app may change only the posts it wrote.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>forum.topic_created</code>, <code>post_created</code>, <code>mentioned</code>, <code>solved</code>, <code>flagged</code>, <code>joined</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Only when you press its button: it summarises a long topic, drafts or polishes your reply, suggests a category and tags, answers a question from what the forum says (with the topics), translates a post, and gives moderators a second opinion. It never posts by itself." >
        <SettingRow title="AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Forum does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own notifications, bookmarks, settings and connections. Admins manage accounts; they do not see private forums they are not in.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account goes, with the forums they own. Their posts in other forums stay, under their name." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [prof, setProf] = useState<ProfileT | null>(null)
  const [busy, setBusy] = useState(false)
  const pic = React.useRef<HTMLInputElement>(null)
  useEffect(() => { api.get<{ profile: ProfileT }>(`${V1}/home`).then(h => setProf(h.profile)).catch(() => {}) }, [])
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      setUser(await api.put(`${V1}/auth/me`, f))
      if (prof) setProf(await api.put<ProfileT>(`${V1}/profile`, { handle: prof.handle, bio: prof.bio, color: prof.color }))
      setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const setPicture = async (file: File | null) => {
    try {
      let id = 0
      if (file) id = (await uploadFile(file, '')).upload.id
      setProf(await api.put<ProfileT>(`${V1}/profile`, { avatar_id: id }))
      toast(file ? 'Picture saved' : 'Picture removed')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Face name={user?.name || user?.email || ''} color={prof?.color} avatarId={prof?.avatar_id} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{prof ? `@${prof.handle} · ` : ''}{user?.email} · {user?.role}</p></div>
        <button type="button" className="btn sm" onClick={() => pic.current?.click()}><Icon name="image" size={14} />Picture</button>
        {!!prof?.avatar_id && <button type="button" className="btn ghost sm" onClick={() => setPicture(null)}>Remove</button>}
        <input ref={pic} type="file" accept="image/*" hidden onChange={e => { const fl = e.target.files?.[0]; e.target.value = ''; if (fl) setPicture(fl) }} />
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name" help="What everyone sees on your posts."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {prof && <Section title="In the forums" sub="The same in every forum you are in.">
        <div className="stack">
          <Field label="Handle" help="People mention you with @ and this. Letters, numbers, dots, dashes."><div className="input-prefix"><span>@</span><input value={prof.handle} onChange={e => setProf({ ...prof, handle: e.target.value.replace(/^@/, '') })} maxLength={30} /></div></Field>
          <Field label="A line about you"><input value={prof.bio} onChange={e => setProf({ ...prof, bio: e.target.value })} maxLength={300} placeholder="Grows tomatoes. Fixes bikes." /></Field>
          <Field label="Colour" group><div className="swatches">{COLORS.map(c => <button key={c} type="button" className={`swatch ${prof.color === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setProf({ ...prof, color: c })} aria-label={c} />)}</div></Field>
        </div>
      </Section>}
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /summary</code></dt><dd className="muted">Unread and new topics, notifications, the latest topics, events and open questions — for Homepage.</dd></div>
          <div><dt><code>GET /topics?filter=unread</code></dt><dd className="muted">Topics with new posts (also <code>new</code>, <code>top</code>, <code>unsolved</code>, <code>events</code>; <code>forum=slug</code>).</dd></div>
          <div><dt><code>POST /topics</code></dt><dd className="muted"><code>{'{"forum": "family", "title": "Dinner on Sunday?", "body": "Who is in?"}'}</code> starts a topic (a Shortcut on the phone).</dd></div>
          <div><dt><code>POST /topics/ID/posts</code></dt><dd className="muted"><code>{'{"body": "On my way"}'}</code> replies.</dd></div>
          <div><dt><code>GET /search?q=…</code></dt><dd className="muted">Search every forum you can read.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">What you wrote on a day.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { toast } = useToast()
  const file = React.useRef<HTMLInputElement>(null)
  const bring = async (fl: File) => {
    try { const r = await upload<{ text: string }>(`${V1}/import`, 'file', fl); toast(r.text) } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Export" icon="archive" sub="The forums you own, whole, as one JSON file (categories, topics, posts and polls; authors by name), with your bookmarks and drafts — or a table of every topic in your forums for a spreadsheet.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Topics as CSV</a></div>
      </Section>
      <Section title="Bring a file in" icon="upload" sub="A Pocket Forum export (also from another box): its forums come back as yours. The same file twice adds nothing twice. To bring a Discourse forum over, open your forum’s settings → Bring topics in.">
        <button type="button" className="btn" onClick={() => file.current?.click()}><Icon name="upload" size={15} />Bring a file…</button>
        <input ref={file} type="file" hidden accept=".json,application/json" onChange={e => { const fl = e.target.files?.[0]; e.target.value = ''; if (fl) bring(fl) }} />
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-forum.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Forums you own', counts.forums, 'forum'], ['Forums you are in', counts.members, 'users'], ['Your posts', counts.posts, 'message'], ['Bookmarks', counts.bookmarks, 'bookmark']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Forum</h3>
          <p className="muted small">Forums for your family, club, team or community: categories, topics and replies with quotes, mentions, reactions, pictures, polls, events and questions with accepted answers; new and unread tracked for you, everything searchable; private, box-wide or public forums with guest posts; a review queue and moderation tools. One Go binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-forum" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Chat, Email, Contacts, Calendar, Tasks, Projects, Notes, Files, Photos, Learn, Blog, Bookshelf, Places, Translate, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}


function TeamPick({ f, setF }: { f: SettingsT; setF: (s: SettingsT) => void }) {
  const on = f.projects_via !== ''
  const { data } = useAsync(() => on ? api.get<{ teams: { id: string; name: string }[] }>(`${V1}/apps/teams`).then(r => r.teams).catch(() => []) : Promise.resolve([]), [on])
  return (
    <SettingRow title="Pocket Projects team" help={on ? 'Where issues made from the forum go first (you can pick another each time).' : 'Connect Pocket Projects above to turn topics into issues.'}>
      <Select ariaLabel="Team" value={String(f.project_team || 0)} onChange={v => setF({ ...f, project_team: Number(v) })} options={[{ value: '0', label: 'Ask each time' }, ...(data || []).map(t => ({ value: String(t.id), label: t.name }))]} />
    </SettingRow>
  )
}
