// Home: the latest, unread, new or top topics of every forum the person is
// in, the events coming up, and a bar when new topics arrive (the list never
// jumps under the reader's finger).
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useLive } from '../lib/live'
import { eventParts, eventWhen } from '../lib/forum'
import type { Home as HomeT, TopicList } from '../lib/types'
import { Icon } from '../components/icons'
import { ForumMark, TopicListView } from '../components/forum'
import { openCreate } from '../components/Layout'
import { Empty, Loading, Segmented, errMsg } from '../components/ui'
import { Select } from '../components/Select'

type Tab = 'latest' | 'unread' | 'new' | 'top'

export default function Home() {
  const { settings } = useApp()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const tab = (sp.get('tab') as Tab) || settings?.home_tab || 'latest'
  const forum = sp.get('forum') || ''
  const period = sp.get('period') || 'month'
  const [data, setData] = useState<TopicList | null>(null)
  const [err, setErr] = useState('')
  const [more, setMore] = useState(0)
  const [page, setPage] = useState(0)
  const [home, setHome] = useState<HomeT | null>(null)
  const [events, setEvents] = useState<TopicList | null>(null)

  const load = useCallback(() => {
    setErr('')
    const q = new URLSearchParams({ filter: tab, period, limit: '30' })
    if (forum) q.set('forum', forum)
    api.get<TopicList>(`${V1}/topics?${q}`).then(d => { setData(d); setMore(0); setPage(0) }).catch(e => setErr(errMsg(e)))
  }, [tab, forum, period])
  useEffect(() => { setData(null); load() }, [load])
  useEffect(() => {
    api.get<HomeT>(`${V1}/home`).then(setHome).catch(() => {})
    api.get<TopicList>(`${V1}/events`).then(setEvents).catch(() => {})
  }, [])
  useLive(e => {
    if (e.type === 'topic' && e.user_id !== home?.profile.user_id && (!forum || data?.forums[String(e.forum_id)])) setMore(n => n + 1)
    if (e.type === 'post' && tab !== 'new' && e.user_id !== home?.profile.user_id) setMore(n => n + 1)
  })
  const loadMore = async () => {
    const q = new URLSearchParams({ filter: tab, period, limit: '30', page: String(page + 1) })
    if (forum) q.set('forum', forum)
    const d = await api.get<TopicList>(`${V1}/topics?${q}`)
    setData(cur => cur ? { ...d, topics: [...cur.topics, ...d.topics.filter(t => !cur.topics.some(x => x.id === t.id))], forums: { ...cur.forums, ...d.forums }, categories: { ...cur.categories, ...d.categories } } : d)
    setPage(p => p + 1)
  }
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }

  const forums = home?.forums.filter(f => f.status === 'active') || []
  if (home && forums.length === 0) {
    return (
      <div className="page narrow">
        <div className="welcome card pad-card">
          <img src="/favicon.svg" alt="" width={56} height={56} />
          <h1>Welcome to Pocket Forum</h1>
          <p className="muted">A calm place to talk things over — for your family, a club, a team or a whole community. Answers stay findable, unlike in a group chat.</p>
          <div className="row wrap">
            <button type="button" className="btn primary lg" onClick={() => openCreate('forum')}><Icon name="plus" size={16} />Make a forum</button>
            {home.discover > 0 && <button type="button" className="btn lg" onClick={() => navigate('/forums')}><Icon name="users" size={16} />{home.discover} {home.discover === 1 ? 'forum' : 'forums'} to join</button>}
          </div>
        </div>
      </div>
    )
  }
  const upcoming = events?.topics.slice(0, 4) || []
  return (
    <div className="page">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{tab === 'unread' ? 'Unread' : tab === 'new' ? 'New topics' : tab === 'top' ? 'Top topics' : 'Latest'}</h1>
          <p className="muted small">{tab === 'unread' ? 'Topics you read before that have new posts.' : tab === 'new' ? 'Topics started since you joined that you have not opened.' : tab === 'top' ? 'What people liked and talked about most.' : 'Everything new in your forums, newest first.'}</p></div>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('topic')}><Icon name="plus" size={15} />New topic</button>
      </div>
      <div className="toolbar row wrap">
        <div className="scroll-x"><Segmented value={tab} onChange={v => set('tab', v === (settings?.home_tab || 'latest') ? '' : v)} options={[
          { value: 'latest', label: 'Latest' }, { value: 'unread', label: <>Unread{home?.unread ? <span className="seg-n">{home.unread}</span> : null}</> },
          { value: 'new', label: <>New{home?.new ? <span className="seg-n">{home.new}</span> : null}</> }, { value: 'top', label: 'Top' }]} /></div>
        {tab === 'top' && <Segmented size="sm" value={period} onChange={v => set('period', v)} options={[{ value: 'week', label: 'Week' }, { value: 'month', label: 'Month' }, { value: 'year', label: 'Year' }, { value: 'all', label: 'All time' }]} />}
        {forums.length > 1 && <div className="tb-forum"><Select ariaLabel="Forum" value={forum} onChange={v => set('forum', v)} options={[{ value: '', label: 'All forums' }, ...forums.map(f => ({ value: f.slug, label: `${f.emoji} ${f.name}` }))]} /></div>}
      </div>

      {upcoming.length > 0 && (tab === 'latest' || !sp.get('tab')) && !forum && (
        <div className="ev-strip">
          {upcoming.map(t => {
            const p = eventParts(t.event_start)
            const f = events!.forums[t.forum_id]
            return <Link key={t.id} to={`/t/${t.id}`} className="ev-card">
              <span className="ev-badge lg"><b>{p.day}</b><small>{p.month}</small></span>
              <span className="grow"><b className="ellipsis">{t.title}</b><span className="muted small ellipsis">{eventWhen(t.event_start)}{t.event_place ? ` · ${t.event_place}` : ''}</span>
                <span className="muted small ellipsis">{f?.emoji} {f?.name}{events!.mine?.[t.id] === 'going' ? ' · you are going' : ''}</span></span>
            </Link>
          })}
          {(events?.topics.length || 0) > 4 && <Link to="/events" className="ev-card more"><Icon name="calendar" size={18} />All events</Link>}
        </div>
      )}

      {more > 0 && <button type="button" className="new-bar" onClick={load}><Icon name="arrow-up" size={15} />{more === 1 ? 'Something new' : `${more} new posts and topics`} — show</button>}
      {err && <p className="error-text">{err}</p>}
      <div className="card flush-card">
        {!data ? <Loading /> : <TopicListView data={data} showForum={!forum} empty={
          <Empty title={tab === 'unread' ? 'You are all caught up' : tab === 'new' ? 'Nothing new since you looked' : 'No topics yet'} icon={tab === 'unread' || tab === 'new' ? 'check-circle' : 'message'}>
            {tab === 'latest' ? <button type="button" className="btn primary" onClick={() => openCreate('topic')}>Start the first topic</button> : <Link to="/">See the latest topics</Link>}
          </Empty>} />}
      </div>
      {data?.more && <div className="row center"><button type="button" className="btn" onClick={loadMore}>Show more</button></div>}
      {forums.length > 0 && !forum && tab === 'latest' && (
        <div className="forum-chips">
          {forums.map(f => <Link key={f.id} to={`/f/${f.slug}`} className="forum-chip"><ForumMark f={f} size={20} />{f.name}{f.unread > 0 && <span className="sb-count">{f.unread}</span>}</Link>)}
        </div>
      )}
    </div>
  )
}
