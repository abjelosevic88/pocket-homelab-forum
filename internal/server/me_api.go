package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// handleHome is the sidebar: the person's forums with what is new, and the
// numbers for the inbox, bookmarks and the review queue.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	prof, _ := s.store.EnsureProfile(ctx, u.ID)
	mine, _ := s.store.MyForums(ctx, u.ID)
	uc := s.store.UnreadCounts(ctx, u.ID, newSince())
	forums := []forumBrief{}
	var mods []int64
	unread, fresh, pendingMembers := 0, 0, 0
	for i := range mine {
		m, err := s.store.GetMember(ctx, mine[i].ID, u.ID)
		if err != nil {
			continue
		}
		b := brief(&mine[i], m)
		if m.Active() {
			b.Unread, b.New = uc[mine[i].ID].Unread, uc[mine[i].ID].New
			unread += b.Unread
			fresh += b.New
		}
		if m.IsMod() {
			mods = append(mods, mine[i].ID)
			b.Pending = s.store.Stats(ctx, mine[i].ID, "").Pending
			pendingMembers += b.Pending
		}
		forums = append(forums, b)
	}
	review := 0
	if len(mods) > 0 {
		pend, _, _ := s.store.PendingPosts(ctx, mods)
		flags, _ := s.store.OpenFlags(ctx, mods)
		seen := map[int64]bool{}
		for _, f := range flags {
			seen[f.PostID] = true
		}
		review = len(pend) + len(seen) + pendingMembers
	}
	bms, _ := s.store.ListBookmarks(ctx, u.ID)
	drafts, _ := s.store.ListDrafts(ctx, u.ID)
	disc, _ := s.store.DiscoverForums(ctx, u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"profile": prof, "forums": forums, "unread": unread, "new": fresh, "inbox": s.store.UnreadNotifications(ctx, u.ID),
		"bookmarks": len(bms), "drafts": len(drafts), "review": review, "moderator": len(mods) > 0, "discover": len(disc),
	})
}

// handleSummary is the short version for Pocket AI, Pocket Day and dashboards.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	_, member, mods := s.readableForums(ctx, u)
	uc := s.store.UnreadCounts(ctx, u.ID, newSince())
	unread, fresh := 0, 0
	for _, c := range uc {
		unread += c.Unread
		fresh += c.New
	}
	latest, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: member, Limit: 8})
	events, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: member, Filter: "events", Limit: 5})
	open, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: member, Filter: "unsolved", Limit: 5})
	fs, _ := s.refs(ctx, u.ID, append(append(append([]store.Topic{}, latest...), events...), open...))
	row := func(t store.Topic) map[string]any {
		return map[string]any{"id": t.ID, "title": t.Title, "forum": fs[t.ForumID].Name, "kind": t.Kind, "replies": t.PostsN - 1, "unread": t.Unread, "new": t.IsNew,
			"last_post_at": t.LastPostAt, "last_by": t.LastName, "event_start": t.EventStart, "event_place": t.EventPlace, "solved": t.SolvedPostID > 0, "url": s.link(fmt.Sprintf("/t/%d", t.ID))}
	}
	rows := func(list []store.Topic) []map[string]any {
		out := []map[string]any{}
		for _, t := range list {
			out = append(out, row(t))
		}
		return out
	}
	review := 0
	if len(mods) > 0 {
		pend, _, _ := s.store.PendingPosts(ctx, mods)
		flags, _ := s.store.OpenFlags(ctx, mods)
		review = len(pend) + len(flags)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"forums": len(member), "unread_topics": unread, "new_topics": fresh, "notifications": s.store.UnreadNotifications(ctx, u.ID), "review": review,
		"latest": rows(latest), "events": rows(events), "open_questions": rows(open), "url": s.link("/"),
	})
}

// handleDay is what the person wrote on one day, for Pocket Day's journal.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	d := chi.URLParam(r, "date")
	day, err := time.ParseInLocation("2006-01-02", d, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	from, to := day.UTC().Format(time.RFC3339Nano), day.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	posts, titles, err := s.store.PostsOn(ctx, u.ID, from, to)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	_, readable, _ := s.readableForums(ctx, u)
	ok := map[int64]bool{}
	for _, id := range readable {
		ok[id] = true
	}
	items := []map[string]any{}
	started := 0
	for i, p := range posts {
		if !ok[p.ForumID] {
			continue
		}
		if p.Number == 1 {
			started++
		}
		items = append(items, map[string]any{"time": p.CreatedAt, "topic": titles[i], "topic_id": p.TopicID, "started": p.Number == 1, "excerpt": excerpt(p.Body, 160),
			"url": s.link(fmt.Sprintf("/t/%d/%d", p.TopicID, p.Number))})
	}
	text := ""
	if n := len(items); n > 0 {
		text = fmt.Sprintf("Wrote %d %s in the forum", n, plural(n, "post", "posts"))
		if started > 0 {
			text += fmt.Sprintf(" (started %d %s)", started, plural(started, "topic", "topics"))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": d, "posts": items, "text": text})
}

// handleSearch searches every forum the person can read.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	q := r.URL.Query()
	sq := store.SearchQuery{Text: q.Get("q"), Tag: q.Get("tag"), Kind: q.Get("kind"), Solved: q.Get("solved"), TitleOnly: queryBool(r, "titles"), Limit: clampInt(queryInt(r, "limit"), 0, 100)}
	if ref := q.Get("forum"); ref != "" {
		a, err := s.forumAccess(ctx, u, ref)
		if err != nil || !a.canRead() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		sq.ForumIDs = []int64{a.f.ID}
		if c := q.Get("category"); c != "" {
			if id, err := strconv.ParseInt(c, 10, 64); err == nil {
				sq.CategoryID = id
			} else if cat, err := s.store.CategoryBySlug(ctx, a.f.ID, c); err == nil {
				sq.CategoryID = cat.ID
			}
		}
	} else {
		all, _, _ := s.readableForums(ctx, u)
		sq.ForumIDs = all
		if queryBool(r, "everywhere") {
			sq.ForumIDs = append(sq.ForumIDs, s.openForums(ctx, u)...)
		}
	}
	if a := q.Get("author"); a != "" {
		if id, err := strconv.ParseInt(a, 10, 64); err == nil {
			sq.AuthorID = id
		} else if p, err := s.store.ProfileByHandle(ctx, a); err == nil {
			sq.AuthorID = p.UserID
		}
	}
	hits, err := s.store.Search(ctx, sq)
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	fs := map[int64]forumMini{}
	for _, h := range hits {
		if _, ok := fs[h.ForumID]; !ok {
			if f, err := s.store.GetForum(ctx, h.ForumID); err == nil {
				fs[f.ID] = forumMini{f.ID, f.Slug, f.Name, f.Emoji, f.Color}
			}
		}
	}
	out := []map[string]any{}
	for _, h := range hits {
		out = append(out, map[string]any{"post_id": h.PostID, "topic_id": h.TopicID, "forum_id": h.ForumID, "number": h.Number, "title": h.Title, "snippet": h.Snippet,
			"author_name": h.AuthorName, "created_at": h.CreatedAt, "kind": h.Kind, "solved": h.Solved, "category_id": h.CategoryID, "tags": h.Tags, "posts_n": h.PostsN,
			"url": s.link(fmt.Sprintf("/t/%d/%d", h.TopicID, h.Number))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": out, "forums": fs})
}

// ---- notifications ----

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	list, err := s.store.ListNotifications(ctx, u.ID, queryBool(r, "unread"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "notifications")
		return
	}
	type row struct {
		store.Notification
		Text string `json:"text"`
		Path string `json:"path"`
	}
	out := make([]row, 0, len(list))
	for _, n := range list {
		out = append(out, row{n, notifTitle(n), notifPath(n)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out, "unread": s.store.UnreadNotifications(ctx, u.ID)})
}

func (s *Server) handleReadNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		IDs []int64 `json:"ids"`
	}
	_ = decode(r, &in)
	if err := s.store.ReadNotifications(ctx, u.ID, in.IDs); err != nil {
		s.fail(w, err, "read")
		return
	}
	n := s.store.UnreadNotifications(ctx, u.ID)
	s.hub.send([]int64{u.ID}, map[string]any{"type": "notification", "unread": n})
	writeJSON(w, http.StatusOK, map[string]any{"unread": n})
}

func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListBookmarks(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "bookmarks")
		return
	}
	for i := range list {
		list[i].Excerpt = excerpt(list[i].Excerpt, 200)
	}
	writeJSON(w, http.StatusOK, list)
}

// ---- drafts ----

var reDraftKey = func(k string) bool {
	parts := strings.SplitN(k, ":", 2)
	if len(parts) != 2 || !oneOf(parts[0], []string{"new", "reply", "edit"}) {
		return false
	}
	_, err := strconv.ParseInt(parts[1], 10, 64)
	return err == nil
}

func (s *Server) handleDrafts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDrafts(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "drafts")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	key := chi.URLParam(r, "key")
	if !reDraftKey(key) {
		writeErr(w, http.StatusBadRequest, "unknown draft")
		return
	}
	switch r.Method {
	case http.MethodGet:
		d, at, err := s.store.GetDraft(ctx, uid, key)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"data": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": d, "updated_at": at})
	case http.MethodPut:
		b, err := s.readBody(r)
		if err != nil || !json.Valid(b) || len(b) > 200000 {
			writeErr(w, http.StatusBadRequest, "invalid draft")
			return
		}
		if err := s.store.SaveDraft(ctx, uid, key, b); err != nil {
			s.fail(w, err, "draft")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		s.store.DeleteDraft(ctx, uid, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- profiles ----

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	p, err := s.store.EnsureProfile(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "profile")
		return
	}
	var in struct {
		Handle   *string `json:"handle"`
		Bio      *string `json:"bio"`
		Color    *string `json:"color"`
		AvatarID *int64  `json:"avatar_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Handle != nil {
		h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(*in.Handle), "@"))
		if !store.ValidHandle(h) {
			writeErr(w, http.StatusBadRequest, "a handle is 2 to 30 letters, numbers, dots, dashes or underscores")
			return
		}
		p.Handle = h
	}
	if in.Bio != nil {
		p.Bio = cleanText(*in.Bio, 300)
	}
	if in.Color != nil {
		if c := strings.TrimSpace(*in.Color); c == "" || reColor.MatchString(c) {
			p.Color = c
		}
	}
	if in.AvatarID != nil {
		if *in.AvatarID > 0 {
			up, err := s.store.GetUpload(ctx, *in.AvatarID)
			if err != nil || up.UserID != u.ID || !strings.HasPrefix(up.Mime, "image/") {
				writeErr(w, http.StatusBadRequest, "pick one of your pictures")
				return
			}
		}
		p.AvatarID = *in.AvatarID
	}
	if err := s.store.UpdateProfile(ctx, p); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "someone already has that handle")
			return
		}
		s.fail(w, err, "profile")
		return
	}
	p, _ = s.store.GetProfile(ctx, u.ID)
	writeJSON(w, http.StatusOK, p)
}

// handlePerson is someone's page: who they are and what they wrote in the
// forums the viewer shares with them.
func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := userFrom(ctx)
	var p *store.Profile
	var err error
	ref := chi.URLParam(r, "id")
	if id, e := strconv.ParseInt(ref, 10, 64); e == nil {
		p, err = s.store.GetProfile(ctx, id)
	} else {
		p, err = s.store.ProfileByHandle(ctx, ref)
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	shared := s.store.SharedForums(ctx, me.ID, p.UserID)
	if p.UserID == me.ID {
		_, shared, _ = s.readableForums(ctx, me)
	}
	if len(shared) == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	forums := []map[string]any{}
	for _, fid := range shared {
		f, err := s.store.GetForum(ctx, fid)
		if err != nil {
			continue
		}
		m, err := s.store.GetMember(ctx, fid, p.UserID)
		if err != nil {
			continue
		}
		forums = append(forums, map[string]any{"forum": forumMini{f.ID, f.Slug, f.Name, f.Emoji, f.Color}, "role": m.Role, "status": m.Status, "title": m.Title,
			"posts": m.PostsN, "topics": m.TopicsN, "likes_got": m.LikesGot, "solutions": m.SolutionsN, "joined_at": m.JoinedAt, "last_seen_at": m.LastSeenAt, "badges": badgesOf(m)})
	}
	posts, titles, _ := s.store.RecentPosts(ctx, p.UserID, shared, 30)
	recent := []map[string]any{}
	for i, x := range posts {
		recent = append(recent, map[string]any{"post_id": x.ID, "topic_id": x.TopicID, "number": x.Number, "title": titles[i], "excerpt": excerpt(x.Body, 220), "created_at": x.CreatedAt, "likes_n": x.LikesN, "forum_id": x.ForumID})
	}
	topics, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: me.ID, ForumIDs: shared, AuthorID: p.UserID, Limit: 20})
	writeJSON(w, http.StatusOK, map[string]any{"profile": p, "forums": forums, "posts": recent, "topics": topics, "me": p.UserID == me.ID})
}
