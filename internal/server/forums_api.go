package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

var reSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

var levels = []string{"watching", "tracking", "normal", "muted"}

// ---- the forum as the page sees it ----

type forumBrief struct {
	ID         int64  `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Emoji      string `json:"emoji"`
	Color      string `json:"color"`
	About      string `json:"about"`
	Visibility string `json:"visibility"`
	JoinPolicy string `json:"join_policy"`
	Archived   bool   `json:"archived"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	Unread     int    `json:"unread"`
	New        int    `json:"new"`
	Members    int    `json:"members"`
	Topics     int    `json:"topics"`
	Pending    int    `json:"pending,omitempty"`
}

func brief(f *store.Forum, m *store.Member) forumBrief {
	b := forumBrief{ID: f.ID, Slug: f.Slug, Name: f.Name, Emoji: f.Emoji, Color: f.Color, About: f.About, Visibility: f.Visibility, JoinPolicy: f.JoinPolicy, Archived: f.Archived}
	if m != nil {
		b.Role, b.Status = m.Role, m.Status
	}
	return b
}

type canDoc struct {
	Read     bool   `json:"read"`
	Post     bool   `json:"post"`
	Why      string `json:"why"`
	Moderate bool   `json:"moderate"`
	Own      bool   `json:"own"`
	Join     string `json:"join"` // "" | open | approval | invite | pending
}

func (a *access) can() canDoc {
	ok, why := a.canPost()
	c := canDoc{Read: a.canRead(), Post: ok, Why: why, Moderate: a.mod(), Own: a.owner()}
	if a.u != nil && (a.m == nil || a.m.Status == "pending") {
		if a.m != nil {
			c.Join = "pending"
		} else if a.f.Visibility != "private" {
			c.Join = a.f.JoinPolicy
		}
	}
	return c
}

// forumDoc is everything a forum page needs.
func (s *Server) forumDoc(ctx context.Context, a *access) map[string]any {
	cats, _ := s.store.ListCategories(ctx, a.f.ID, a.uid())
	if !a.mod() {
		keep := cats[:0]
		for _, c := range cats {
			if !c.Archived || c.Topics > 0 {
				keep = append(keep, c)
			}
		}
		cats = keep
	}
	mods, _ := s.store.ListMembers(ctx, a.f.ID, "active", "", 50)
	staff := []map[string]any{}
	for _, m := range mods {
		if m.Role == "owner" || m.Role == "moderator" {
			staff = append(staff, map[string]any{"user_id": m.UserID, "name": m.Name, "handle": m.Handle, "role": m.Role, "color": m.Color, "avatar_id": m.Avatar})
		}
	}
	f := *a.f
	if !a.mod() {
		f.BlockedWords, f.HeldWords = []string{}, []string{}
	}
	doc := map[string]any{"forum": f, "reactions": a.f.ReactionList(), "categories": cats, "staff": staff, "can": a.can(),
		"stats":    s.store.Stats(ctx, a.f.ID, time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339Nano)),
		"feed_url": "", "public_url": ""}
	if a.f.Visibility == "public" {
		doc["public_url"] = s.link("/p/" + a.f.Slug)
		doc["feed_url"] = s.link("/feeds/" + a.f.Slug + ".rss")
	}
	if a.m != nil {
		doc["me"] = a.m
	}
	if a.mod() {
		pend, _, _ := s.store.PendingPosts(ctx, []int64{a.f.ID})
		flags, _ := s.store.OpenFlags(ctx, []int64{a.f.ID})
		flagged := map[int64]bool{}
		for _, fl := range flags {
			flagged[fl.PostID] = true
		}
		doc["review"] = map[string]int{"posts": len(pend), "flags": len(flagged), "members": s.store.Stats(ctx, a.f.ID, "").Pending}
	}
	if a.u != nil && a.canRead() {
		uc := s.store.UnreadCounts(ctx, a.u.ID, newSince())
		doc["unread"] = uc[a.f.ID]
	}
	return doc
}

func newSince() string { return time.Now().AddDate(0, 0, -14).UTC().Format(time.RFC3339Nano) }

// handleForums lists the person's forums and the ones they could join.
func (s *Server) handleForums(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	mine, err := s.store.MyForums(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "forums")
		return
	}
	uc := s.store.UnreadCounts(ctx, u.ID, newSince())
	since := time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339Nano)
	out := []forumBrief{}
	for i := range mine {
		m, _ := s.store.GetMember(ctx, mine[i].ID, u.ID)
		b := brief(&mine[i], m)
		b.Unread, b.New = uc[mine[i].ID].Unread, uc[mine[i].ID].New
		st := s.store.Stats(ctx, mine[i].ID, since)
		b.Members, b.Topics = st.Members, st.Topics
		if m.IsMod() {
			b.Pending = st.Pending
		}
		out = append(out, b)
	}
	disc, _ := s.store.DiscoverForums(ctx, u.ID)
	open := []forumBrief{}
	for i := range disc {
		b := brief(&disc[i], nil)
		st := s.store.Stats(ctx, disc[i].ID, since)
		b.Members, b.Topics = st.Members, st.Topics
		open = append(open, b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"forums": out, "discover": open, "templates": forumTemplates})
}

func (s *Server) handleGetForum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if a.member() {
		s.store.TouchMember(r.Context(), a.f.ID, a.uid())
	}
	writeJSON(w, http.StatusOK, s.forumDoc(r.Context(), a))
}

type forumIn struct {
	Name         *string   `json:"name"`
	Slug         *string   `json:"slug"`
	About        *string   `json:"about"`
	Emoji        *string   `json:"emoji"`
	Color        *string   `json:"color"`
	Visibility   *string   `json:"visibility"`
	JoinPolicy   *string   `json:"join_policy"`
	GuestPosts   *bool     `json:"guest_posts"`
	ApproveFirst *int      `json:"approve_first"`
	Rules        *string   `json:"rules"`
	Welcome      *string   `json:"welcome"`
	SlowMode     *int      `json:"slow_mode"`
	EditMinutes  *int      `json:"edit_minutes"`
	BlockedWords *[]string `json:"blocked_words"`
	HeldWords    *[]string `json:"held_words"`
	Reactions    *[]string `json:"reactions"`
	Archived     *bool     `json:"archived"`
	OwnerID      *int64    `json:"owner_id"`
	Template     string    `json:"template"`
}

func wordList(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, w := range in {
		for _, part := range strings.Split(w, "\n") {
			p := strings.ToLower(strings.TrimSpace(part))
			if p != "" && !seen[p] && len(out) < 500 {
				seen[p] = true
				out = append(out, clip(p, 60))
			}
		}
	}
	return out
}

// apply copies what was sent; owner-only fields need owner.
func (in forumIn) apply(f *store.Forum, owner bool) error {
	if in.Name != nil {
		n := cleanText(*in.Name, 80)
		if utf8.RuneCountInString(n) < 2 {
			return &userErr{"give the forum a name"}
		}
		f.Name = n
	}
	if in.About != nil {
		f.About = cleanText(*in.About, 300)
	}
	if in.Emoji != nil {
		f.Emoji = firstNonEmpty(clip(strings.TrimSpace(*in.Emoji), 8), "💬")
	}
	if in.Color != nil {
		if c := strings.TrimSpace(*in.Color); c == "" || reColor.MatchString(c) {
			f.Color = c
		}
	}
	if in.Rules != nil {
		f.Rules = clip(strings.TrimSpace(*in.Rules), 20000)
	}
	if in.Welcome != nil {
		f.Welcome = clip(strings.TrimSpace(*in.Welcome), 4000)
	}
	if in.SlowMode != nil {
		f.SlowMode = clampInt(*in.SlowMode, 0, 86400)
	}
	if in.EditMinutes != nil {
		f.EditMinutes = clampInt(*in.EditMinutes, 0, 525600)
	}
	if in.ApproveFirst != nil {
		f.ApproveFirst = clampInt(*in.ApproveFirst, 0, 20)
	}
	if in.BlockedWords != nil {
		f.BlockedWords = wordList(*in.BlockedWords)
	}
	if in.HeldWords != nil {
		f.HeldWords = wordList(*in.HeldWords)
	}
	if in.Reactions != nil {
		var re []string
		for _, e := range *in.Reactions {
			if e = strings.TrimSpace(e); e != "" && utf8.RuneCountInString(e) <= 4 && len(re) < 8 && !oneOf(e, re) {
				re = append(re, e)
			}
		}
		f.Reactions = re
	}
	if !owner {
		if in.Visibility != nil || in.JoinPolicy != nil || in.GuestPosts != nil || in.Archived != nil || in.OwnerID != nil || in.Slug != nil {
			return &userErr{"only the forum's owner can change who may see and join it, its address, or archive it"}
		}
		return nil
	}
	if in.Visibility != nil {
		if !oneOf(*in.Visibility, []string{"private", "box", "public"}) {
			return &userErr{"a forum is private, open to this box, or public"}
		}
		f.Visibility = *in.Visibility
	}
	if in.JoinPolicy != nil {
		if !oneOf(*in.JoinPolicy, []string{"open", "approval", "invite"}) {
			return &userErr{"people join freely, by asking, or by invitation"}
		}
		f.JoinPolicy = *in.JoinPolicy
	}
	if f.Visibility == "private" {
		f.JoinPolicy = "invite"
	}
	if in.GuestPosts != nil {
		f.GuestPosts = *in.GuestPosts
	}
	if f.Visibility != "public" {
		f.GuestPosts = false
	}
	if in.Archived != nil {
		f.Archived = *in.Archived
	}
	if in.Slug != nil {
		sl := strings.ToLower(strings.TrimSpace(*in.Slug))
		if !reSlug.MatchString(sl) {
			return &userErr{"the address is lowercase letters, numbers and dashes"}
		}
		f.Slug = sl
	}
	return nil
}

func (s *Server) handleCreateForum(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	if linkApp(ctx) != "" {
		writeErr(w, http.StatusForbidden, "other apps cannot make forums")
		return
	}
	var in forumIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	tpl := templateByID(in.Template)
	f := &store.Forum{UserID: u.ID, Emoji: tpl.Emoji, Visibility: tpl.Visibility, JoinPolicy: tpl.JoinPolicy, Rules: tpl.Rules, Welcome: tpl.Welcome, Reactions: []string{}}
	if in.Name == nil {
		writeErr(w, http.StatusBadRequest, "give the forum a name")
		return
	}
	if err := in.apply(f, true); err != nil {
		s.userFail(w, err, "forum")
		return
	}
	if in.Slug == nil {
		f.Slug = store.Slugify(f.Name, "-")
		if len(f.Slug) > 40 {
			f.Slug = strings.Trim(f.Slug[:40], "-")
		}
	}
	if !reSlug.MatchString(f.Slug) {
		f.Slug = "forum"
	}
	f.Slug = s.store.FreeForumSlug(ctx, f.Slug, 0)
	if err := s.store.CreateForum(ctx, f, tpl.Categories); err != nil {
		s.fail(w, err, "create forum")
		return
	}
	_, _ = s.store.EnsureProfile(ctx, u.ID)
	a, _ := s.accessTo(ctx, u, f)
	s.modLog(ctx, a, "created the forum", "forum", f.ID, f.Name)
	writeJSON(w, http.StatusCreated, s.forumDoc(ctx, a))
}

func (s *Server) handleUpdateForum(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	var in forumIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := *a.f
	if err := in.apply(&f, a.owner()); err != nil {
		s.userFail(w, err, "forum")
		return
	}
	if in.OwnerID != nil && *in.OwnerID != f.UserID {
		m, err := s.store.GetMember(ctx, f.ID, *in.OwnerID)
		if err != nil || !m.Active() {
			writeErr(w, http.StatusBadRequest, "the new owner has to be a member of the forum")
			return
		}
		old, _ := s.store.GetMember(ctx, f.ID, f.UserID)
		m.Role = "owner"
		_ = s.store.UpdateMember(ctx, m)
		if old != nil {
			old.Role = "moderator"
			_ = s.store.UpdateMember(ctx, old)
		}
		f.UserID = *in.OwnerID
		s.modLog(ctx, a, "handed the forum to", "member", m.UserID, m.Name)
	}
	if f.Slug != a.f.Slug && s.store.FreeForumSlug(ctx, f.Slug, f.ID) != f.Slug {
		writeErr(w, http.StatusConflict, "another forum already has that address")
		return
	}
	if err := s.store.UpdateForum(ctx, &f); err != nil {
		s.fail(w, err, "save forum")
		return
	}
	s.modLog(ctx, a, "changed the forum's settings", "forum", f.ID, "")
	a.f = &f
	if m, err := s.store.GetMember(ctx, f.ID, a.uid()); err == nil {
		a.m = m
	}
	s.live(f.ID, map[string]any{"type": "forum"})
	writeJSON(w, http.StatusOK, s.forumDoc(ctx, a))
}

func (s *Server) handleDeleteForum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if !a.owner() || linkApp(r.Context()) != "" {
		writeErr(w, http.StatusForbidden, "only the forum's owner can delete it")
		return
	}
	if err := s.store.DeleteForum(r.Context(), a.f.ID); err != nil {
		s.fail(w, err, "delete forum")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- joining ----

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if a.m != nil {
		switch a.m.Status {
		case "banned":
			writeErr(w, http.StatusForbidden, "you are banned from this forum"+untilWords(a.m.BannedUntil))
		default:
			writeJSON(w, http.StatusOK, s.forumDoc(ctx, a))
		}
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = decode(r, &in)
	switch a.f.JoinPolicy {
	case "open":
		if err := s.join(ctx, a.f, u, "member", 0); err != nil {
			s.fail(w, err, "join")
			return
		}
	case "approval":
		m := &store.Member{ForumID: a.f.ID, UserID: u.ID, Status: "pending", Note: cleanText(in.Note, 500)}
		if err := s.store.AddMember(ctx, m); err != nil {
			s.fail(w, err, "join")
			return
		}
		s.tellMods(ctx, a.f, store.Notification{Kind: "join_request", ActorID: u.ID, ActorName: s.actorName(u), Title: a.f.Name, Excerpt: m.Note}, 0)
	default:
		writeErr(w, http.StatusForbidden, "this forum takes new members by invitation only")
		return
	}
	a, _ = s.accessTo(ctx, u, a.f)
	writeJSON(w, http.StatusOK, s.forumDoc(ctx, a))
}

// join puts a person in a forum as an active member and welcomes them.
func (s *Server) join(ctx context.Context, f *store.Forum, u *store.User, role string, invitedBy int64) error {
	if err := s.store.AddMember(ctx, &store.Member{ForumID: f.ID, UserID: u.ID, Role: role, Status: "active", InvitedBy: invitedBy}); err != nil {
		return err
	}
	_, _ = s.store.EnsureProfile(ctx, u.ID)
	if f.Welcome != "" {
		s.addNotification(ctx, &store.Notification{UserID: u.ID, ForumID: f.ID, Kind: "welcome", Title: f.Name, Excerpt: excerpt(f.Welcome, 200)})
	}
	s.store.Log(ctx, store.LogEntry{ForumID: f.ID, UserID: u.ID, ActorName: s.actorName(u), Action: "joined", TargetKind: "member", TargetID: u.ID, Detail: role})
	s.publish(ctx, u.ID, "forum.joined", f.Name, map[string]any{"forum": f.Slug, "url": s.link("/f/" + f.Slug)})
	return nil
}

func (s *Server) handleLeave(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if a.m == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if a.m.Role == "owner" {
		writeErr(w, http.StatusBadRequest, "hand the forum to someone else first (Forum settings → Members), or delete it")
		return
	}
	if a.m.Status == "banned" {
		writeErr(w, http.StatusBadRequest, "a ban stays until it ends")
		return
	}
	if err := s.store.RemoveMember(r.Context(), a.f.ID, a.uid()); err != nil {
		s.fail(w, err, "leave")
		return
	}
	s.modLog(r.Context(), a, "left", "member", a.uid(), "")
	w.WriteHeader(http.StatusNoContent)
}

// handleMyMembership saves how the person follows a forum and whether it is in their summary email.
func (s *Server) handleMyMembership(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if a.m == nil {
		writeErr(w, http.StatusBadRequest, "join the forum first")
		return
	}
	var in struct {
		Level  *string `json:"level"`
		Digest *bool   `json:"digest"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Level != nil {
		if !oneOf(*in.Level, levels) {
			writeErr(w, http.StatusBadRequest, "watching, tracking, normal or muted")
			return
		}
		a.m.Level = *in.Level
	}
	if in.Digest != nil {
		a.m.Digest = *in.Digest
	}
	if err := s.store.UpdateMember(r.Context(), a.m); err != nil {
		s.fail(w, err, "save")
		return
	}
	writeJSON(w, http.StatusOK, a.m)
}

// ---- members ----

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if !a.mod() {
		status = "active"
		if !a.member() {
			writeErr(w, http.StatusForbidden, "join the forum to see who is in it")
			return
		}
	}
	list, err := s.store.ListMembers(r.Context(), a.f.ID, status, r.URL.Query().Get("q"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "members")
		return
	}
	if !a.mod() {
		for i := range list {
			list[i].Note, list[i].BanReason = "", ""
		}
	}
	type row struct {
		store.Member
		Badges []badge `json:"badges"`
	}
	out := make([]row, 0, len(list))
	for _, m := range list {
		out = append(out, row{m, badgesOf(&m)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

// handlePeople suggests members for an @mention.
func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if !a.member() {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	list, err := s.store.ListMembers(r.Context(), a.f.ID, "active", r.URL.Query().Get("q"), 8)
	if err != nil {
		s.fail(w, err, "people")
		return
	}
	out := []map[string]any{}
	for _, m := range list {
		out = append(out, map[string]any{"user_id": m.UserID, "name": m.Name, "handle": m.Handle, "color": m.Color, "avatar_id": m.Avatar})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAddMember puts someone who already has an account on this box in the forum.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "nobody on this box has that email: send them an invitation instead")
		return
	}
	if m, err := s.store.GetMember(ctx, a.f.ID, u.ID); err == nil && m.Status == "banned" {
		writeErr(w, http.StatusBadRequest, "that person is banned: lift the ban first")
		return
	}
	role := "member"
	if in.Role == "moderator" && a.owner() {
		role = "moderator"
	}
	if err := s.join(ctx, a.f, u, role, a.uid()); err != nil {
		s.fail(w, err, "add member")
		return
	}
	s.addNotification(ctx, &store.Notification{UserID: u.ID, ForumID: a.f.ID, Kind: "joined", ActorID: a.uid(), ActorName: s.actorName(a.u), Title: a.f.Name})
	m, _ := s.store.GetMember(ctx, a.f.ID, u.ID)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	m, err := s.store.GetMember(ctx, a.f.ID, idParam(r, "uid"))
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	var in struct {
		Role         *string `json:"role"`
		Status       *string `json:"status"`
		Title        *string `json:"title"`
		BanDays      *int    `json:"ban_days"` // 0 = for good
		BanReason    *string `json:"ban_reason"`
		SilenceHours *int    `json:"silence_hours"` // 0 = lift
		Approve      *bool   `json:"approve"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if m.Role == "owner" && (in.Role != nil || in.Status != nil || in.SilenceHours != nil) {
		writeErr(w, http.StatusBadRequest, "the owner cannot be changed here")
		return
	}
	if m.Role == "moderator" && !a.owner() && (in.Role != nil || in.Status != nil || in.SilenceHours != nil) {
		writeErr(w, http.StatusForbidden, "only the owner can change a moderator")
		return
	}
	user, _ := s.store.GetUser(ctx, m.UserID)
	if in.Approve != nil && m.Status == "pending" {
		if *in.Approve {
			if err := s.join(ctx, a.f, user, "member", a.uid()); err != nil {
				s.fail(w, err, "approve")
				return
			}
			s.addNotification(ctx, &store.Notification{UserID: m.UserID, ForumID: a.f.ID, Kind: "joined", ActorID: a.uid(), ActorName: s.actorName(a.u), Title: a.f.Name})
			s.modLog(ctx, a, "let in", "member", m.UserID, m.Name)
		} else {
			_ = s.store.RemoveMember(ctx, a.f.ID, m.UserID)
			s.modLog(ctx, a, "turned down", "member", m.UserID, m.Name)
		}
		m, _ = s.store.GetMember(ctx, a.f.ID, m.UserID)
		writeJSON(w, http.StatusOK, map[string]any{"member": m})
		return
	}
	if in.Role != nil {
		if !a.owner() {
			writeErr(w, http.StatusForbidden, "only the owner chooses the moderators")
			return
		}
		if !oneOf(*in.Role, []string{"moderator", "member"}) {
			writeErr(w, http.StatusBadRequest, "moderator or member")
			return
		}
		if m.Role != *in.Role {
			s.modLog(ctx, a, "made "+*in.Role, "member", m.UserID, m.Name)
		}
		m.Role = *in.Role
	}
	if in.Title != nil {
		m.Title = cleanText(*in.Title, 40)
	}
	if in.Status != nil {
		switch *in.Status {
		case "banned":
			if m.UserID == a.uid() {
				writeErr(w, http.StatusBadRequest, "you cannot ban yourself")
				return
			}
			m.Status, m.Role = "banned", "member"
			m.BannedUntil = nil
			if in.BanDays != nil && *in.BanDays > 0 {
				at := time.Now().AddDate(0, 0, *in.BanDays).UTC().Format(time.RFC3339Nano)
				m.BannedUntil = &at
			}
			if in.BanReason != nil {
				m.BanReason = cleanText(*in.BanReason, 300)
			}
			s.modLog(ctx, a, "banned", "member", m.UserID, m.Name+untilWords(m.BannedUntil)+" — "+m.BanReason)
			s.addNotification(ctx, &store.Notification{UserID: m.UserID, ForumID: a.f.ID, Kind: "moderation", ActorID: a.uid(), ActorName: s.actorName(a.u),
				Title: "You are banned from " + a.f.Name + untilWords(m.BannedUntil), Excerpt: m.BanReason})
		case "active":
			if m.Status == "banned" {
				s.modLog(ctx, a, "lifted the ban on", "member", m.UserID, m.Name)
			}
			m.Status, m.BannedUntil, m.BanReason = "active", nil, ""
		default:
			writeErr(w, http.StatusBadRequest, "active or banned")
			return
		}
	}
	if in.SilenceHours != nil {
		if *in.SilenceHours > 0 {
			at := time.Now().Add(time.Duration(*in.SilenceHours) * time.Hour).UTC().Format(time.RFC3339Nano)
			m.SilencedUntil = &at
			s.modLog(ctx, a, "paused posting for", "member", m.UserID, fmt.Sprintf("%s for %d h", m.Name, *in.SilenceHours))
			s.addNotification(ctx, &store.Notification{UserID: m.UserID, ForumID: a.f.ID, Kind: "moderation", ActorID: a.uid(), ActorName: s.actorName(a.u),
				Title: "A moderator paused your posting in " + a.f.Name + untilWords(m.SilencedUntil)})
		} else {
			m.SilencedUntil = nil
		}
	}
	if err := s.store.UpdateMember(ctx, m); err != nil {
		s.fail(w, err, "save member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": m})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	m, err := s.store.GetMember(ctx, a.f.ID, idParam(r, "uid"))
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	if m.Role == "owner" || (m.Role == "moderator" && !a.owner()) {
		writeErr(w, http.StatusForbidden, "you cannot remove them")
		return
	}
	if err := s.store.RemoveMember(ctx, a.f.ID, m.UserID); err != nil {
		s.fail(w, err, "remove")
		return
	}
	s.modLog(ctx, a, "removed", "member", m.UserID, m.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---- invites ----

func (s *Server) inviteDoc(i store.Invite) map[string]any {
	return map[string]any{"invite": i, "url": s.link("/join/" + i.Token)}
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListInvites(r.Context(), a.f.ID)
	if err != nil {
		s.fail(w, err, "invites")
		return
	}
	out := []map[string]any{}
	for _, i := range list {
		out = append(out, s.inviteDoc(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Note     string `json:"note"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		MaxUses  int    `json:"max_uses"`
		Days     int    `json:"days"`
		Accounts *bool  `json:"accounts"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	i := &store.Invite{ForumID: a.f.ID, UserID: a.uid(), Token: newToken(), Note: cleanText(in.Note, 200), Email: strings.ToLower(strings.TrimSpace(in.Email)),
		Role: "member", MaxUses: clampInt(in.MaxUses, 0, 10000), Accounts: s.cfg.AuthMode == "local"}
	if in.Role == "moderator" && a.owner() {
		i.Role = "moderator"
	}
	if in.Accounts != nil && !*in.Accounts {
		i.Accounts = false
	}
	if in.Days > 0 {
		at := time.Now().AddDate(0, 0, clampInt(in.Days, 1, 365)).UTC().Format(time.RFC3339Nano)
		i.ExpiresAt = &at
	}
	if i.Email != "" && i.MaxUses == 0 {
		i.MaxUses = 1
	}
	if err := s.store.CreateInvite(ctx, i); err != nil {
		s.fail(w, err, "invite")
		return
	}
	s.modLog(ctx, a, "made an invite link", "invite", i.ID, firstNonEmpty(i.Email, i.Note))
	writeJSON(w, http.StatusCreated, s.inviteDoc(*i))
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	i, err := s.store.GetInvite(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	a, err := s.forumAccess(ctx, userFrom(ctx), fmt.Sprint(i.ForumID))
	if err != nil || !a.mod() {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	_ = s.store.RevokeInvite(ctx, i.ID)
	s.modLog(ctx, a, "stopped an invite link", "invite", i.ID, firstNonEmpty(i.Email, i.Note))
	w.WriteHeader(http.StatusNoContent)
}

// inviteUsable says why an invite cannot be used ("" = it can).
func inviteUsable(i *store.Invite) string {
	switch {
	case i.Revoked:
		return "this invitation was withdrawn"
	case i.ExpiresAt != nil && *i.ExpiresAt < store.Now():
		return "this invitation has run out"
	case i.MaxUses > 0 && i.Uses >= i.MaxUses:
		return "this invitation has been used"
	}
	return ""
}

// handleAcceptInvite joins the signed-in person with an invite link.
func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	i, err := s.store.InviteByToken(ctx, chi.URLParam(r, "token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invitation does not exist")
		return
	}
	f, err := s.store.GetForum(ctx, i.ForumID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invitation does not exist")
		return
	}
	if m, err := s.store.GetMember(ctx, f.ID, u.ID); err == nil {
		if m.Status == "banned" {
			writeErr(w, http.StatusForbidden, "you are banned from this forum")
			return
		}
		if m.Status == "active" {
			writeJSON(w, http.StatusOK, map[string]any{"slug": f.Slug, "already": true})
			return
		}
	}
	if why := inviteUsable(i); why != "" {
		writeErr(w, http.StatusGone, why)
		return
	}
	if i.Email != "" && !strings.EqualFold(i.Email, u.Email) {
		writeErr(w, http.StatusForbidden, "this invitation is for "+i.Email)
		return
	}
	if err := s.join(ctx, f, u, i.Role, i.UserID); err != nil {
		s.fail(w, err, "join")
		return
	}
	s.store.UseInvite(ctx, i.ID)
	writeJSON(w, http.StatusOK, map[string]any{"slug": f.Slug})
}

// ---- categories ----

type categoryIn struct {
	Name     *string `json:"name"`
	Slug     *string `json:"slug"`
	About    *string `json:"about"`
	Emoji    *string `json:"emoji"`
	Color    *string `json:"color"`
	ParentID *int64  `json:"parent_id"`
	Starts   *string `json:"starts"`
	QA       *bool   `json:"qa"`
	Archived *bool   `json:"archived"`
}

func (s *Server) applyCategory(ctx context.Context, fid int64, in categoryIn, c *store.Category) error {
	if in.Name != nil {
		c.Name = cleanText(*in.Name, 60)
	}
	if c.Name == "" {
		return &userErr{"give the category a name"}
	}
	if in.Slug != nil || c.Slug == "" {
		sl := store.Slugify(c.Name, "-")
		if in.Slug != nil && *in.Slug != "" {
			sl = strings.ToLower(strings.TrimSpace(*in.Slug))
		}
		if !reSlug.MatchString(sl) {
			sl = fmt.Sprintf("category-%d", time.Now().Unix()%10000)
		}
		base := sl
		for i := 2; ; i++ {
			other, err := s.store.CategoryBySlug(ctx, fid, sl)
			if err != nil || other.ID == c.ID {
				break
			}
			sl = fmt.Sprintf("%s-%d", base, i)
		}
		c.Slug = sl
	}
	if in.About != nil {
		c.About = cleanText(*in.About, 300)
	}
	if in.Emoji != nil {
		c.Emoji = clip(strings.TrimSpace(*in.Emoji), 8)
	}
	if in.Color != nil {
		if v := strings.TrimSpace(*in.Color); v == "" || reColor.MatchString(v) {
			c.Color = v
		}
	}
	if in.ParentID != nil {
		c.ParentID = *in.ParentID
		if c.ParentID == c.ID {
			c.ParentID = 0
		}
		if c.ParentID > 0 {
			p, err := s.store.GetCategory(ctx, c.ParentID)
			if err != nil || p.ForumID != fid || p.ParentID > 0 {
				return &userErr{"a category can sit inside a top-level category of the same forum"}
			}
		}
	}
	if in.Starts != nil {
		if !oneOf(*in.Starts, []string{"all", "mods"}) {
			return &userErr{"everyone or moderators"}
		}
		c.Starts = *in.Starts
	}
	if c.Starts == "" {
		c.Starts = "all"
	}
	if in.QA != nil {
		c.QA = *in.QA
	}
	if in.Archived != nil {
		c.Archived = *in.Archived
	}
	return nil
}

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	cats, err := s.store.ListCategories(r.Context(), a.f.ID, a.uid())
	if err != nil {
		s.fail(w, err, "categories")
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	var in categoryIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := &store.Category{ForumID: a.f.ID}
	if err := s.applyCategory(ctx, a.f.ID, in, c); err != nil {
		s.userFail(w, err, "category")
		return
	}
	if err := s.store.SaveCategory(ctx, a.uid(), c); err != nil {
		s.fail(w, err, "category")
		return
	}
	s.modLog(ctx, a, "added the category", "category", c.ID, c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) categoryParam(w http.ResponseWriter, r *http.Request) (*access, *store.Category, bool) {
	ctx := r.Context()
	c, err := s.store.GetCategory(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	a, err := s.forumAccess(ctx, userFrom(ctx), fmt.Sprint(c.ForumID))
	if err != nil || !a.canRead() {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return a, c, true
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, c, ok := s.categoryParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	var in categoryIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.applyCategory(ctx, a.f.ID, in, c); err != nil {
		s.userFail(w, err, "category")
		return
	}
	if err := s.store.SaveCategory(ctx, a.uid(), c); err != nil {
		s.fail(w, err, "category")
		return
	}
	s.modLog(ctx, a, "changed the category", "category", c.ID, c.Name)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, c, ok := s.categoryParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	to := int64(queryInt(r, "move_to"))
	if to > 0 {
		if t, err := s.store.GetCategory(ctx, to); err != nil || t.ForumID != a.f.ID || t.ID == c.ID {
			writeErr(w, http.StatusBadRequest, "move the topics to another category of this forum")
			return
		}
	}
	if err := s.store.DeleteCategory(ctx, c.ID, to); err != nil {
		s.fail(w, err, "delete category")
		return
	}
	s.modLog(ctx, a, "removed the category", "category", c.ID, c.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderCategories(w http.ResponseWriter, r *http.Request) {
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderCategories(r.Context(), a.f.ID, in.IDs); err != nil {
		s.fail(w, err, "order")
		return
	}
	cats, _ := s.store.ListCategories(r.Context(), a.f.ID, a.uid())
	writeJSON(w, http.StatusOK, cats)
}

func (s *Server) handleCategoryLevel(w http.ResponseWriter, r *http.Request) {
	a, c, ok := s.categoryParam(w, r)
	if !ok {
		return
	}
	if !a.member() {
		writeErr(w, http.StatusBadRequest, "join the forum first")
		return
	}
	var in struct {
		Level string `json:"level"`
	}
	if err := decode(r, &in); err != nil || (in.Level != "" && !oneOf(in.Level, levels)) {
		writeErr(w, http.StatusBadRequest, "watching, tracking, normal, muted or empty")
		return
	}
	if err := s.store.SetCategoryLevel(r.Context(), c.ID, a.uid(), in.Level); err != nil {
		s.fail(w, err, "level")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"level": in.Level})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	tags, err := s.store.Tags(r.Context(), a.f.ID)
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleModLog(w http.ResponseWriter, r *http.Request) {
	a, ok := s.modParam(w, r)
	if !ok {
		return
	}
	list, err := s.store.ModLog(r.Context(), a.f.ID, 300)
	if err != nil {
		s.fail(w, err, "log")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ---- badges ----

type badge struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Line  string `json:"line"`
}

// badgesOf are what a member earned in a forum, worked out from their numbers.
func badgesOf(m *store.Member) []badge {
	out := []badge{}
	add := func(ok bool, id, name, emoji, line string) {
		if ok {
			out = append(out, badge{id, name, emoji, line})
		}
	}
	add(m.Role == "owner", "founder", "Founder", "🏛️", "Started this forum")
	add(m.Role == "moderator", "moderator", "Moderator", "🛡️", "Keeps this forum friendly")
	add(m.PostsN >= 1, "first_post", "First post", "✍️", "Wrote a first post")
	add(m.PostsN >= 50, "regular", "Regular", "🗣️", "50 posts or more")
	add(m.PostsN >= 250, "pillar", "Pillar", "🏛", "250 posts or more")
	add(m.SolutionsN >= 1, "helpful", "Helpful", "✅", "Wrote an accepted answer")
	add(m.SolutionsN >= 10, "guru", "Guru", "🎓", "10 accepted answers")
	add(m.LikesGot >= 10, "liked", "Liked", "❤️", "10 reactions to their posts")
	add(m.LikesGot >= 100, "loved", "Loved", "💖", "100 reactions to their posts")
	add(m.LikesGiven >= 25, "cheerleader", "Cheerleader", "📣", "Reacted 25 times")
	add(m.TopicsN >= 10, "conversation_starter", "Conversation starter", "💡", "Started 10 topics")
	if at, err := time.Parse(time.RFC3339Nano, m.JoinedAt); err == nil {
		add(time.Since(at) > 365*24*time.Hour, "anniversary", "Anniversary", "🎂", "A member for a year")
	}
	return out
}
