import type { AppSlug, AppStateName, Level, TopicKind, Visibility, JoinPolicy } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  chat: { name: 'Pocket Chat', icon: 'message' },
  email: { name: 'Pocket Email', icon: 'mail' },
  contacts: { name: 'Pocket Contacts', icon: 'users' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square' },
  projects: { name: 'Pocket Projects', icon: 'layers' },
  notes: { name: 'Pocket Notes', icon: 'note' },
  files: { name: 'Pocket Files', icon: 'file' },
  photos: { name: 'Pocket Photos', icon: 'image' },
  learn: { name: 'Pocket Learn', icon: 'brain' },
  blog: { name: 'Pocket Blog', icon: 'feather' },
  bookshelf: { name: 'Pocket Bookshelf', icon: 'book-open' },
  places: { name: 'Pocket Places', icon: 'pin' },
  translate: { name: 'Pocket Translate', icon: 'globe' },
}

export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Off', link_off: 'Link is off in Pocket Core', missing: 'Not on this box', old: 'Needs an update',
  no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not working',
}

export const KINDS: { value: TopicKind; label: string; icon: string; line: string }[] = [
  { value: 'discussion', label: 'Discussion', icon: 'message', line: 'Talk something over' },
  { value: 'question', label: 'Question', icon: 'help', line: 'Ask, and mark the reply that answers it' },
  { value: 'event', label: 'Event', icon: 'calendar', line: 'A date and a place; people say whether they come' },
  { value: 'announcement', label: 'Announcement', icon: 'alert', line: 'News for everyone (moderators)' },
]

export const LEVELS: { value: Level; label: string; icon: string; line: string }[] = [
  { value: 'watching', label: 'Watching', icon: 'bell', line: 'Tell me about every new post' },
  { value: 'tracking', label: 'Tracking', icon: 'eye', line: 'Count new posts; tell me when someone mentions or answers me' },
  { value: 'normal', label: 'Normal', icon: 'circle', line: 'Tell me only when someone mentions or answers me' },
  { value: 'muted', label: 'Muted', icon: 'bell-off', line: 'Never; hide it from my lists' },
]
export const LEVEL: Record<string, (typeof LEVELS)[number]> = Object.fromEntries(LEVELS.map(l => [l.value, l]))

export const VISIBILITY: Record<Visibility, { label: string; icon: string; line: string }> = {
  private: { label: 'Private', icon: 'lock', line: 'Only the people you invite see it' },
  box: { label: 'Everyone here', icon: 'users', line: 'Everyone with an account on this box can find and read it' },
  public: { label: 'Public', icon: 'globe', line: 'Anyone with the link can read it, without an account' },
}

export const JOIN: Record<JoinPolicy, { label: string; line: string }> = {
  open: { label: 'Anyone can join', line: 'One tap and they are in' },
  approval: { label: 'Ask to join', line: 'A moderator lets people in' },
  invite: { label: 'By invitation', line: 'Only with an invite link' },
}

export const FLAG_REASONS = [
  { value: 'spam', label: 'Spam', line: 'An advert, or something posted only to sell' },
  { value: 'rude', label: 'Rude or unkind', line: 'A personal attack, insults or worse' },
  { value: 'off_topic', label: 'Off topic', line: 'It belongs in another topic or category' },
  { value: 'other', label: 'Something else', line: 'Tell the moderators what is wrong' },
]

export const COLORS = ['#4361c2', '#0f766e', '#15803d', '#a16207', '#c2410c', '#be123c', '#a83b6c', '#7c3aed', '#2563eb', '#6b7280']

export const EMOJIS = ['💬', '🏠', '🏡', '⚽', '📚', '🧭', '🛟', '🎮', '🎸', '🌱', '🍲', '🏘️', '🧑‍💻', '🚲', '🏔️', '📷', '🎨', '🐾', '✈️', '❤️']

export function forumColor(c?: string) { return c || 'var(--accent)' }

export function avatarURL(id: number, pub = false) { return id ? `/api/v1/${pub ? 'public/' : ''}uploads/${id}?thumb=1` : '' }

export function topicPath(id: number, n?: number) { return n && n > 1 ? `/t/${id}/${n}` : `/t/${id}` }

export function eventWhen(start: string, end?: string): string {
  if (!start) return ''
  const hasTime = start.length > 10
  const d = new Date(hasTime ? start : start + 'T00:00')
  const day = d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: d.getFullYear() !== new Date().getFullYear() ? 'numeric' : undefined })
  if (!hasTime) return day
  const t = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  let out = `${day}, ${t}`
  if (end && end.length > 10) {
    const e = new Date(end)
    out += '–' + e.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  }
  return out
}

export function eventParts(start: string) {
  const d = new Date(start.length > 10 ? start : start + 'T00:00')
  return { month: d.toLocaleDateString(undefined, { month: 'short' }).toUpperCase(), day: d.getDate() }
}

export const LANGS = [
  { value: 'en', label: 'English' }, { value: 'bs', label: 'Bosnian' }, { value: 'hr', label: 'Croatian' }, { value: 'sr', label: 'Serbian' }, { value: 'de', label: 'German' },
  { value: 'fr', label: 'French' }, { value: 'es', label: 'Spanish' }, { value: 'it', label: 'Italian' }, { value: 'tr', label: 'Turkish' }, { value: 'nl', label: 'Dutch' },
]

export function myLang(): string {
  const l = (navigator.language || 'en').slice(0, 2)
  return LANGS.some(x => x.value === l) ? l : 'en'
}

/** "3 replies", "1 reply" */
export function replies(n: number) { return n === 1 ? '1 reply' : `${n.toLocaleString()} replies` }

export function compact(n: number): string {
  if (n < 1000) return String(n)
  if (n < 10000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  return Math.round(n / 1000) + 'k'
}
