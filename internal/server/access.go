package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// Who may do what in a forum.
//
//   - private: only its active members see it (anyone else gets 404, the
//     box's admins too).
//   - box: everyone with an account here may read it; members post.
//   - public: anyone with the link reads it, without an account.
//
// Posting needs an active membership that is not silenced; moderators (and
// the owner) may also post in closed topics and "moderators only" categories.

type access struct {
	f *store.Forum
	m *store.Member // nil = not a member
	u *store.User   // nil = nobody signed in (public pages)
}

func (a *access) member() bool { return a.m.Active() }
func (a *access) mod() bool    { return a.m.IsMod() }
func (a *access) owner() bool  { return a.m.Active() && a.m.Role == "owner" }
func (a *access) uid() int64 {
	if a.u == nil {
		return 0
	}
	return a.u.ID
}

func (a *access) canRead() bool {
	if a.member() {
		return true
	}
	switch a.f.Visibility {
	case "public":
		return true
	case "box":
		return a.u != nil
	}
	return false
}

// silenced is true while a member may read but not post.
func (a *access) silenced() bool {
	return a.m != nil && a.m.SilencedUntil != nil && *a.m.SilencedUntil > store.Now()
}

// canPost says whether the person may write in this forum at all, and why not.
func (a *access) canPost() (bool, string) {
	switch {
	case a.u == nil:
		return false, "sign in to post"
	case a.m == nil || a.m.Status == "pending":
		if a.m != nil {
			return false, "your request to join is waiting for a moderator"
		}
		return false, "join the forum to post"
	case a.m.Status == "banned":
		return false, "you are banned from this forum" + untilWords(a.m.BannedUntil)
	case a.silenced():
		return false, "a moderator has paused your posting" + untilWords(a.m.SilencedUntil)
	case a.f.Archived && !a.mod():
		return false, "this forum is archived: read only"
	}
	return true, ""
}

func untilWords(at *string) string {
	if at == nil || *at == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		return ""
	}
	return " until " + t.Local().Format("2 Jan 2006 15:04")
}

func (a *access) canStart(c *store.Category) (bool, string) {
	if ok, why := a.canPost(); !ok {
		return false, why
	}
	if c != nil && c.Starts == "mods" && !a.mod() {
		return false, "only moderators start topics in " + c.Name
	}
	if c != nil && c.Archived && !a.mod() {
		return false, c.Name + " is archived"
	}
	return true, ""
}

func (a *access) canReply(t *store.Topic) (bool, string) {
	if ok, why := a.canPost(); !ok {
		return false, why
	}
	if a.mod() {
		return true, ""
	}
	switch {
	case t.Archived:
		return false, "this topic is archived"
	case t.Closed:
		return false, "this topic is closed"
	}
	return true, ""
}

// canEditPost: the author (inside the forum's edit window), anyone in a
// wiki post, a moderator always.
func (a *access) canEditPost(p *store.Post, t *store.Topic) bool {
	if a.mod() {
		return true
	}
	if ok, _ := a.canPost(); !ok || t.Archived {
		return false
	}
	if p.Wiki && p.State == "visible" {
		return true
	}
	if a.u == nil || p.UserID != a.u.ID || p.State == "deleted" {
		return false
	}
	if a.f.EditMinutes > 0 {
		at, err := time.Parse(time.RFC3339Nano, p.CreatedAt)
		if err == nil && time.Since(at) > time.Duration(a.f.EditMinutes)*time.Minute {
			return false
		}
	}
	return true
}

// canSeeTopic: visible topics for readers; pending/hidden ones for their
// author and the moderators.
func (a *access) canSeeTopic(t *store.Topic) bool {
	if !a.canRead() {
		return false
	}
	switch t.State {
	case "visible":
		return true
	case "pending", "hidden":
		return a.mod() || (a.u != nil && t.UserID == a.u.ID)
	case "deleted":
		return a.mod()
	}
	return false
}

var errNoForum = errors.New("not found")

// forumAccess loads a forum by slug or id for the person (nil = public) and
// refuses one they may not read with ErrNotFound.
func (s *Server) forumAccess(ctx context.Context, u *store.User, ref string) (*access, error) {
	var f *store.Forum
	var err error
	if id, e := strconv.ParseInt(ref, 10, 64); e == nil && id > 0 {
		f, err = s.store.GetForum(ctx, id)
	} else {
		f, err = s.store.GetForumBySlug(ctx, ref)
	}
	if err != nil {
		return nil, store.ErrNotFound
	}
	return s.accessTo(ctx, u, f)
}

func (s *Server) accessTo(ctx context.Context, u *store.User, f *store.Forum) (*access, error) {
	a := &access{f: f, u: u}
	if u != nil {
		if m, err := s.store.GetMember(ctx, f.ID, u.ID); err == nil {
			// a ban or a pause that ran out ends by itself
			now := store.Now()
			changed := false
			if m.Status == "banned" && m.BannedUntil != nil && *m.BannedUntil != "" && *m.BannedUntil <= now {
				m.Status, m.BannedUntil, m.BanReason, changed = "active", nil, "", true
			}
			if m.SilencedUntil != nil && *m.SilencedUntil <= now {
				m.SilencedUntil, changed = nil, true
			}
			if changed {
				_ = s.store.UpdateMember(ctx, m)
			}
			a.m = m
		}
	}
	if !a.canRead() && !(a.m != nil && a.m.Status == "pending") {
		return nil, store.ErrNotFound
	}
	return a, nil
}

// forumParam is the forum named in the URL, for the person signed in.
func (s *Server) forumParam(w http.ResponseWriter, r *http.Request) (*access, bool) {
	a, err := s.forumAccess(r.Context(), userFrom(r.Context()), chi.URLParam(r, "forum"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if a.m != nil && a.m.Status == "pending" && !a.canRead() {
		writeErr(w, http.StatusForbidden, "your request to join is waiting for a moderator")
		return nil, false
	}
	return a, true
}

// modParam is forumParam for the moderators only.
func (s *Server) modParam(w http.ResponseWriter, r *http.Request) (*access, bool) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return nil, false
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return nil, false
	}
	return a, true
}

// topicAccess loads a topic and the person's access to its forum.
func (s *Server) topicAccess(ctx context.Context, u *store.User, id int64) (*access, *store.Topic, error) {
	t, err := s.store.GetTopic(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	f, err := s.store.GetForum(ctx, t.ForumID)
	if err != nil {
		return nil, nil, err
	}
	a, err := s.accessTo(ctx, u, f)
	if err != nil || !a.canSeeTopic(t) {
		return nil, nil, store.ErrNotFound
	}
	return a, t, nil
}

func (s *Server) topicParam(w http.ResponseWriter, r *http.Request) (*access, *store.Topic, bool) {
	a, t, err := s.topicAccess(r.Context(), userFrom(r.Context()), idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return a, t, true
}

// postParam loads a post the person can see, with its topic and forum.
func (s *Server) postParam(w http.ResponseWriter, r *http.Request) (*access, *store.Topic, *store.Post, bool) {
	ctx := r.Context()
	p, err := s.store.GetPost(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, nil, false
	}
	a, t, err := s.topicAccess(ctx, userFrom(ctx), p.TopicID)
	if err != nil || !canSeePost(a, p) {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, nil, false
	}
	return a, t, p, true
}

func canSeePost(a *access, p *store.Post) bool {
	switch p.State {
	case "visible":
		return true
	case "deleted":
		return true // shown as "removed"
	}
	return a.mod() || (a.u != nil && p.UserID == a.u.ID)
}

// readableForums are the ids of every forum the person may read, and the
// ones they moderate.
func (s *Server) readableForums(ctx context.Context, u *store.User) (all []int64, member []int64, mods []int64) {
	mine, _ := s.store.MyForums(ctx, u.ID)
	seen := map[int64]bool{}
	for _, f := range mine {
		m, err := s.store.GetMember(ctx, f.ID, u.ID)
		if err != nil {
			continue
		}
		a := &access{f: &f, m: m, u: u}
		if !a.canRead() {
			continue
		}
		seen[f.ID] = true
		all = append(all, f.ID)
		if m.Active() {
			member = append(member, f.ID)
		}
		if m.IsMod() {
			mods = append(mods, f.ID)
		}
	}
	return all, member, mods
}

// openForums are the forums the person can read without being in them.
func (s *Server) openForums(ctx context.Context, u *store.User) []int64 {
	list, _ := s.store.DiscoverForums(ctx, u.ID)
	var out []int64
	for _, f := range list {
		out = append(out, f.ID)
	}
	return out
}

func (s *Server) actorName(u *store.User) string {
	if u == nil {
		return ""
	}
	return firstNonEmpty(u.Name, strings.Split(u.Email, "@")[0])
}

// log writes a moderation step by the person.
func (s *Server) modLog(ctx context.Context, a *access, action, kind string, id int64, detail string) {
	s.store.Log(ctx, store.LogEntry{ForumID: a.f.ID, UserID: a.uid(), ActorName: s.actorName(a.u), Action: action, TargetKind: kind, TargetID: id, Detail: detail})
}
