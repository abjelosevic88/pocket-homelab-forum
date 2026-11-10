package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Live updates: every open page keeps one event stream; new posts, topics
// and notifications reach the people in that forum straight away. A person
// with a page open counts as "here" and gets no phone push for what they see.

type hub struct {
	mu   sync.Mutex
	subs map[int64]map[chan []byte]bool
}

func newHub() *hub { return &hub{subs: map[int64]map[chan []byte]bool{}} }

func (h *hub) sub(uid int64) chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	if h.subs[uid] == nil {
		h.subs[uid] = map[chan []byte]bool{}
	}
	h.subs[uid][ch] = true
	h.mu.Unlock()
	return ch
}

func (h *hub) unsub(uid int64, ch chan []byte) {
	h.mu.Lock()
	delete(h.subs[uid], ch)
	if len(h.subs[uid]) == 0 {
		delete(h.subs, uid)
	}
	h.mu.Unlock()
}

// online is true while the person has a page open.
func (h *hub) online(uid int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[uid]) > 0
}

// send delivers an event to these people (a full buffer drops it: the page
// catches up on its next load).
func (h *hub) send(uids []int64, ev map[string]any) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, uid := range uids {
		for ch := range h.subs[uid] {
			select {
			case ch <- b:
			default:
			}
		}
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming is not supported here")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := s.hub.sub(u.ID)
	defer s.hub.unsub(u.ID, ch)
	fmt.Fprintf(w, "event: hello\ndata: {\"unread\":%d}\n\n", s.store.UnreadNotifications(r.Context(), u.ID))
	fl.Flush()
	beat := time.NewTicker(25 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// live tells the forum's members about a change.
func (s *Server) live(fid int64, ev map[string]any) {
	go func() {
		ids := s.store.MemberIDs(contextBG(), fid, false)
		ev["forum_id"] = fid
		s.hub.send(ids, ev)
	}()
}
