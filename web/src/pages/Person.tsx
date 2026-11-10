// Someone's page: who they are, their badges in each forum you share with
// them, what they wrote lately and the topics they started.
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ago, fmtDate } from '../lib/format'
import type { Badge, ForumMini, Profile, Topic } from '../lib/types'
import { Icon } from '../components/icons'
import { Face, ForumMark, TopicListView } from '../components/forum'
import { useCrumb } from '../components/Layout'
import { Empty, Loading, Tabs } from '../components/ui'

interface PersonDoc {
  profile: Profile; me: boolean
  forums: { forum: ForumMini; role: string; status: string; title: string; posts: number; topics: number; likes_got: number; solutions: number; joined_at: string; last_seen_at: string | null; badges: Badge[] }[]
  posts: { post_id: number; topic_id: number; number: number; title: string; excerpt: string; created_at: string; likes_n: number; forum_id: number }[]
  topics: Topic[]
}

export default function Person() {
  const { id = '' } = useParams()
  const { user } = useApp()
  const ref = id === 'me' ? String(user?.id || '') : id
  const [doc, setDoc] = useState<PersonDoc | null>(null)
  const [err, setErr] = useState('')
  const [tab, setTab] = useState<'posts' | 'topics' | 'badges'>('posts')
  useEffect(() => { setDoc(null); api.get<PersonDoc>(`${V1}/people/${encodeURIComponent(ref)}`).then(setDoc).catch(() => setErr('not found')) }, [ref])
  useCrumb(doc?.profile.name)
  if (err) return <div className="page narrow"><Empty title="Nobody by that name here" icon="user">You only see the people you share a forum with.</Empty></div>
  if (!doc) return <Loading />
  const p = doc.profile
  const total = doc.forums.reduce((a, f) => ({ posts: a.posts + f.posts, likes: a.likes + f.likes_got, sol: a.sol + f.solutions }), { posts: 0, likes: 0, sol: 0 })
  const fmap = Object.fromEntries(doc.forums.map(f => [f.forum.id, f.forum]))
  const badges = doc.forums.flatMap(f => f.badges.map(b => ({ ...b, forum: f.forum })))
  return (
    <div className="page narrow">
      <header className="person-head card pad-card">
        <Face name={p.name} color={p.color} avatarId={p.avatar_id} size={72} />
        <div className="grow">
          <h1 className="page-title">{p.name}</h1>
          <p className="muted">@{p.handle}</p>
          {p.bio && <p>{p.bio}</p>}
          <div className="chips mt-s"><span className="chip-s"><Icon name="message" size={12} />{total.posts} posts</span><span className="chip-s"><Icon name="heart" size={12} />{total.likes} reactions</span><span className="chip-s"><Icon name="check-circle" size={12} />{total.sol} solutions</span></div>
        </div>
        {doc.me && <Link to="/settings/profile" className="btn"><Icon name="edit" size={15} />Edit</Link>}
      </header>
      <div className="stack tight">{doc.forums.map(f => (
        <Link key={f.forum.id} to={`/f/${f.forum.slug}`} className="row person-forum"><ForumMark f={f.forum} size={26} /><span className="grow"><b>{f.forum.name}</b>{f.role !== 'member' && <span className={`role-pill ${f.role}`}>{f.role === 'owner' ? 'Owner' : 'Moderator'}</span>}{f.title && <span className="role-pill">{f.title}</span>}{f.status === 'banned' && <span className="role-pill banned">banned</span>}</span>
          <span className="muted small">{f.posts} posts · joined {fmtDate(f.joined_at.slice(0, 10))}{f.last_seen_at ? ` · seen ${ago(f.last_seen_at)}` : ''}</span></Link>
      ))}</div>
      <Tabs tabs={[{ id: 'posts', label: 'Posts' }, { id: 'topics', label: 'Topics' }, { id: 'badges', label: `Badges (${badges.length})` }]} value={tab} onChange={setTab} />
      {tab === 'posts' && <div className="card flush-card">{doc.posts.length === 0 ? <Empty title="No posts yet" icon="message" /> : <div className="rows">{doc.posts.map(x => (
        <Link key={x.post_id} to={`/t/${x.topic_id}/${x.number}`} className="row"><span className="grow"><b>{x.title}</b><span className="muted small ellipsis">{x.excerpt}</span><span className="muted small">{fmap[x.forum_id]?.emoji} {fmap[x.forum_id]?.name} · {ago(x.created_at)}{x.likes_n ? ` · ${x.likes_n} reactions` : ''}</span></span></Link>
      ))}</div>}</div>}
      {tab === 'topics' && <div className="card flush-card"><TopicListView data={{ topics: doc.topics, forums: fmap, categories: {}, more: false }} showForum empty={<Empty title="No topics yet" icon="message" />} /></div>}
      {tab === 'badges' && (badges.length === 0 ? <div className="card"><Empty title="No badges yet" icon="trophy">Badges come with posting, helping and being around.</Empty></div> :
        <div className="badge-grid">{badges.map((b, i) => <div key={i} className="badge-card"><span className="badge-emoji" aria-hidden="true">{b.emoji}</span><b>{b.name}</b><span className="muted small">{b.line}</span><span className="muted small">{b.forum.emoji} {b.forum.name}</span></div>)}</div>)}
    </div>
  )
}
