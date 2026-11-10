// One live connection per tab: new posts, topics and notifications arrive as
// they happen. Pages listen with useLive; the bell reads useUnread.
import React, { createContext, useContext, useEffect, useRef, useState } from 'react'
import { V1 } from './api'

export interface LiveEvent {
  type: 'post' | 'topic' | 'edit' | 'reaction' | 'poll' | 'rsvp' | 'notification' | 'review' | 'forum'
  forum_id?: number; topic_id?: number; post_id?: number; number?: number; user_id?: number; category_id?: number; unread?: number; kind?: string
}

type Listener = (e: LiveEvent) => void
const listeners = new Set<Listener>()

const Ctx = createContext<{ unread: number; setUnread: (n: number) => void; connected: boolean }>({ unread: 0, setUnread: () => {}, connected: false })
export function useUnread() { return useContext(Ctx) }

export function LiveProvider({ children }: { children: React.ReactNode }) {
  const [unread, setUnread] = useState(0)
  const [connected, setConnected] = useState(false)
  const retry = useRef(1000)
  useEffect(() => {
    let es: EventSource | null = null
    let timer = 0
    let stopped = false
    const open = () => {
      if (stopped) return
      es = new EventSource(`${V1}/stream`, { withCredentials: true })
      es.addEventListener('hello', e => {
        retry.current = 1000
        setConnected(true)
        try { setUnread(JSON.parse((e as MessageEvent).data).unread || 0) } catch { /* ignore */ }
      })
      es.onmessage = e => {
        try {
          const ev = JSON.parse(e.data) as LiveEvent
          if (ev.type === 'notification' && typeof ev.unread === 'number') setUnread(ev.unread)
          listeners.forEach(l => l(ev))
        } catch { /* ignore */ }
      }
      es.onerror = () => {
        setConnected(false)
        es?.close()
        timer = window.setTimeout(open, retry.current)
        retry.current = Math.min(retry.current * 2, 30000)
      }
    }
    open()
    return () => { stopped = true; es?.close(); window.clearTimeout(timer) }
  }, [])
  return <Ctx.Provider value={{ unread, setUnread, connected }}>{children}</Ctx.Provider>
}

/** Call fn for every live event while the component is mounted. */
export function useLive(fn: Listener) {
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => {
    const l: Listener = e => ref.current(e)
    listeners.add(l)
    return () => { listeners.delete(l) }
  }, [])
}
