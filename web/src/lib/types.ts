export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'chat' | 'email' | 'contacts' | 'calendar' | 'tasks' | 'projects' | 'notes' | 'files' | 'photos' | 'learn' | 'blog' | 'bookshelf' | 'places' | 'translate'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  home_tab: 'latest' | 'unread' | 'new' | 'top'; density: 'comfortable' | 'compact'; auto_watch: boolean; preview: boolean; load_images: boolean
  notes_folder: string; files_folder: string; chat_to: string; learn_deck: string; project_team: number; ai_help: boolean
  notify_mentions: boolean; notify_replies: boolean; notify_watching: boolean; notify_new_topics: boolean; notify_review: boolean; notify_likes: boolean
  notify_hour: number; digest: 'off' | 'daily' | 'weekly'; digest_day: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

// ---- forums ----

export type Visibility = 'private' | 'box' | 'public'
export type JoinPolicy = 'open' | 'approval' | 'invite'
export type Level = 'watching' | 'tracking' | 'normal' | 'muted'
export type Role = 'owner' | 'moderator' | 'member'
export type TopicKind = 'discussion' | 'question' | 'announcement' | 'event'

export interface Profile { user_id: number; handle: string; name: string; bio: string; color: string; avatar_id: number }

export interface Forum {
  id: number; owner_id: number; slug: string; name: string; about: string; emoji: string; color: string; visibility: Visibility; join_policy: JoinPolicy
  guest_posts: boolean; approve_first: number; rules: string; welcome: string; slow_mode: number; edit_minutes: number
  blocked_words: string[]; held_words: string[]; reactions: string[]; archived: boolean; source: string; created_at: string; updated_at: string
}

export interface ForumBrief {
  id: number; slug: string; name: string; emoji: string; color: string; about: string; visibility: Visibility; join_policy: JoinPolicy; archived: boolean
  role: Role | ''; status: string; unread: number; new: number; members: number; topics: number; pending?: number
}

export interface ForumMini { id: number; slug: string; name: string; emoji: string; color: string }

export interface Member {
  forum_id: number; user_id: number; role: Role; status: 'active' | 'pending' | 'banned'; title: string; note: string; level: Level; digest: boolean
  banned_until: string | null; ban_reason: string; silenced_until: string | null; invited_by: number
  posts_n: number; topics_n: number; likes_given: number; likes_got: number; solutions_n: number; joined_at: string; last_seen_at: string | null; last_post_at: string | null
  name: string; handle: string; color: string; avatar_id: number; badges?: Badge[]
}

export interface Badge { id: string; name: string; emoji: string; line: string }

export interface Category {
  id: number; forum_id: number; parent_id: number; name: string; slug: string; about: string; emoji: string; color: string; position: number
  starts: 'all' | 'mods'; qa: boolean; archived: boolean; created_at: string; topics: number; posts: number; latest_at: string; level: Level | ''
}
export interface CatMini { id: number; name: string; slug: string; emoji: string; color: string; parent_id: number; qa: boolean }

export interface Can { read: boolean; post: boolean; why: string; moderate: boolean; own: boolean; join: '' | JoinPolicy | 'pending' }

export interface ForumDoc {
  forum: Forum; reactions: string[]; categories: Category[]; staff: { user_id: number; name: string; handle: string; role: Role; color: string; avatar_id: number }[]
  can: Can; stats: { topics: number; posts: number; members: number; pending_members: number; week_topics: number; week_posts: number; solved: number }
  me?: Member; review?: { posts: number; flags: number; members: number }; unread?: { unread: number; new: number }
  feed_url: string; public_url: string; guest_posts?: boolean
}

export interface Poster { id: number; name: string; color?: string }

export interface Topic {
  id: number; forum_id: number; category_id: number; user_id: number; author_name: string; title: string; kind: TopicKind; state: string
  closed: boolean; archived: boolean; pinned: number; pinned_until: string | null; unlisted: boolean; solved_post_id: number
  event_start: string; event_end: string; event_place: string; close_at: string | null; slow_mode: number; moved_to: number; tags: string[]; excerpt: string
  views: number; posts_n: number; highest: number; likes_n: number; posters: Poster[]; last_post_at: string | null; last_user_id: number; last_name: string
  bumped_at: string; source: string; source_ref: string; created_at: string; updated_at: string
  last_read: number; unread: number; is_new: boolean; level: Level | ''; visited: boolean
}

export interface TopicList { topics: Topic[]; forums: Record<string, ForumMini>; categories: Record<string, CatMini>; more: boolean; mine?: Record<string, string> }

export interface Poll {
  id: number; post_id: number; topic_id: number; question: string; options: string[]; multi: boolean; public: boolean; closes_at: string | null; closed: boolean
  counts: number[]; voters: number; mine: number[]; who?: string[][]; user_id: number
}

export interface Post {
  id: number; topic_id: number; forum_id: number; user_id: number; author_name: string; number: number; reply_to: number; body: string; state: string; hidden_reason: string
  wiki: boolean; edits: number; edited_at: string | null; likes_n: number; reactions: Record<string, number>; source: string; created_at: string; updated_at: string
  mine: string[]; handle: string; color: string; avatar_id: number; role: Role | ''; title: string; replies: number; poll?: Poll; bookmarked: boolean
}

export interface RSVP { user_id: number; name: string; status: 'going' | 'maybe' | 'no' }

export interface TopicDoc {
  topic: Topic; posts: Post[]; category: CatMini | null
  forum: { id: number; slug: string; name: string; emoji: string; color: string; visibility: Visibility; reactions: string[]; edit_minutes: number }
  can: { reply: boolean; why: string; moderate: boolean; member: boolean; edit_topic: boolean; solve: boolean; join: string }
  rsvps?: RSVP[]; public_url?: string; guest_posts?: boolean; moved_to?: number
}

export interface Notification {
  id: number; forum_id: number; kind: string; topic_id: number; post_id: number; number: number; actor_id: number; actor_name: string; title: string; excerpt: string
  read_at: string | null; created_at: string; forum_slug: string; forum_name: string; forum_emoji: string; text: string; path: string
}

export interface Bookmark {
  id: number; post_id: number; note: string; remind_at: string | null; reminded: boolean; created_at: string; topic_id: number; topic_title: string; number: number
  excerpt: string; author_name: string; forum_slug: string; forum_name: string; forum_emoji: string
}

export interface Draft { key: string; data: { title?: string; body?: string; forum?: string; category_id?: number }; updated_at: string }

export interface Home {
  profile: Profile; forums: ForumBrief[]; unread: number; new: number; inbox: number; bookmarks: number; drafts: number; review: number; moderator: boolean; discover: number
}

export interface Hit {
  post_id: number; topic_id: number; forum_id: number; number: number; title: string; snippet: string; author_name: string; created_at: string
  kind: TopicKind; solved: boolean; category_id: number; tags: string[]; posts_n: number; url: string
}

export interface Invite { id: number; forum_id: number; token: string; note: string; email: string; role: Role; max_uses: number; uses: number; expires_at: string | null; accounts: boolean; revoked: boolean; created_at: string }

export interface LogEntry { id: number; user_id: number; actor_name: string; action: string; target_kind: string; target_id: number; detail: string; created_at: string }

export interface Flag { id: number; post_id: number; user_id: number; reason: string; note: string; status: string; created_at: string; flagger: string }

export interface Template { id: string; name: string; line: string; emoji: string; visibility: Visibility; join_policy: JoinPolicy; categories: { name: string; emoji: string }[] }
