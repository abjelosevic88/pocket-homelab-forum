package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// The review queue: everything waiting for a moderator in the forums the
// person moderates — held posts, flagged posts and people asking to join.

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	_, _, mods := s.readableForums(ctx, u)
	fs := map[int64]forumMini{}
	for _, id := range mods {
		if f, err := s.store.GetForum(ctx, id); err == nil {
			fs[id] = forumMini{f.ID, f.Slug, f.Name, f.Emoji, f.Color}
		}
	}
	pend, titles, err := s.store.PendingPosts(ctx, mods)
	if err != nil {
		s.fail(w, err, "review")
		return
	}
	held := []map[string]any{}
	for i, p := range pend {
		held = append(held, map[string]any{"post": p, "topic_title": titles[i], "guest": p.UserID == 0, "guest_email": p.GuestEmail, "new_topic": p.Number == 1,
			"why": s.heldWhy(r, p)})
	}
	flags, err := s.store.OpenFlags(ctx, mods)
	if err != nil {
		s.fail(w, err, "review")
		return
	}
	byPost := map[int64][]store.Flag{}
	order := []int64{}
	for _, f := range flags {
		if byPost[f.PostID] == nil {
			order = append(order, f.PostID)
		}
		byPost[f.PostID] = append(byPost[f.PostID], f)
	}
	flagged := []map[string]any{}
	for _, pid := range order {
		p, err := s.store.GetPost(ctx, pid)
		if err != nil {
			continue
		}
		t, _ := s.store.GetTopic(ctx, p.TopicID)
		title := ""
		if t != nil {
			title = t.Title
		}
		flagged = append(flagged, map[string]any{"post": p, "topic_title": title, "flags": byPost[pid]})
	}
	joins := []map[string]any{}
	for _, id := range mods {
		list, _ := s.store.ListMembers(ctx, id, "pending", "", 200)
		for _, m := range list {
			joins = append(joins, map[string]any{"member": m, "forum_id": id})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"held": held, "flagged": flagged, "joins": joins, "forums": fs, "moderator": len(mods) > 0})
}

func (s *Server) heldWhy(r *http.Request, p store.Post) string {
	if p.UserID == 0 {
		return "A guest wrote it"
	}
	f, err := s.store.GetForum(r.Context(), p.ForumID)
	if err == nil {
		if w := containsWord(p.Body, f.HeldWords); w != "" {
			return "It has a word on the watch list (“" + w + "”)"
		}
		if f.ApproveFirst > 0 {
			return fmt.Sprintf("New members' first %d %s wait for a moderator", f.ApproveFirst, plural(f.ApproveFirst, "post", "posts"))
		}
	}
	return "Held for a moderator"
}

// handleReviewPost approves or rejects a held post.
func (s *Server) handleReviewPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	var in struct {
		Action string `json:"action"` // approve | reject
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Action, []string{"approve", "reject"}) {
		writeErr(w, http.StatusBadRequest, "approve or reject")
		return
	}
	if p.State != "pending" {
		writeErr(w, http.StatusConflict, "someone already dealt with it")
		return
	}
	if in.Action == "reject" {
		_ = s.store.SetPostState(ctx, p, "deleted", "rejected")
		if p.Number == 1 {
			t.State = "deleted"
			_ = s.store.UpdateTopic(ctx, t)
		}
		s.modLog(ctx, a, "rejected a held post", "post", p.ID, t.Title)
		if p.UserID > 0 {
			s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "moderation", TopicID: t.ID, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: "A moderator did not approve your post in “" + t.Title + "”"})
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if p.Number == 1 && t.State == "pending" {
		t.State = "visible"
		_ = s.store.UpdateTopic(ctx, t)
	}
	if err := s.store.SetPostState(ctx, p, "visible", ""); err != nil {
		s.fail(w, err, "approve")
		return
	}
	s.modLog(ctx, a, "approved a held post", "post", p.ID, t.Title)
	if p.UserID > 0 {
		s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "moderation", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: "Your post in “" + t.Title + "” is up"})
	}
	s.afterPost(ctx, a, t, p, p.Number == 1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleReviewFlags settles the flags on a post: agree (hide the post),
// disagree (keep it), or delete the post.
func (s *Server) handleReviewFlags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	var in struct {
		Action string `json:"action"` // agree | disagree | ignore | delete
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Action, []string{"agree", "disagree", "ignore", "delete"}) {
		writeErr(w, http.StatusBadRequest, "agree, disagree, ignore or delete")
		return
	}
	status := map[string]string{"agree": "agreed", "disagree": "disagreed", "ignore": "ignored", "delete": "agreed"}[in.Action]
	switch in.Action {
	case "agree":
		_ = s.store.SetPostState(ctx, p, "hidden", "flagged by the community")
		if p.Number == 1 {
			t.State = "hidden"
			_ = s.store.UpdateTopic(ctx, t)
		}
		if p.UserID > 0 {
			s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "moderation", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: "A moderator hid your post in “" + t.Title + "”"})
		}
	case "delete":
		_ = s.store.SetPostState(ctx, p, "deleted", "")
		if p.Number == 1 {
			t.State = "deleted"
			_ = s.store.UpdateTopic(ctx, t)
		}
	}
	if err := s.store.HandleFlags(ctx, p.ID, status, a.uid()); err != nil {
		s.fail(w, err, "flags")
		return
	}
	s.modLog(ctx, a, map[string]string{"agree": "agreed with a flag and hid the post", "disagree": "kept a flagged post", "ignore": "ignored a flag", "delete": "removed a flagged post"}[in.Action],
		"post", p.ID, t.Title+" #"+strconv.Itoa(p.Number))
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
