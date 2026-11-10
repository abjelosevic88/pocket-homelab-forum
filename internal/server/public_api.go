package server

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// Public forums: anyone with the link reads them; when the owner allows
// it, people without an account post as guests (always held for a
// moderator). Invitation links work here too, for people who have no
// account on this box yet.

// publicForum is a public forum, for nobody in particular.
func (s *Server) publicForum(w http.ResponseWriter, r *http.Request) (*access, bool) {
	f, err := s.store.GetForumBySlug(r.Context(), chi.URLParam(r, "forum"))
	if err != nil || f.Visibility != "public" {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return &access{f: f}, true
}

func (s *Server) handlePublicForum(w http.ResponseWriter, r *http.Request) {
	a, ok := s.publicForum(w, r)
	if !ok {
		return
	}
	doc := s.forumDoc(r.Context(), a)
	doc["guest_posts"] = a.f.GuestPosts
	delete(doc, "me")
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handlePublicTopics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.publicForum(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	tq := store.TopicQuery{ForumIDs: []int64{a.f.ID}, Filter: q.Get("filter"), Tag: q.Get("tag"), Limit: 30, Since: periodSince(firstNonEmpty(q.Get("period"), "month"))}
	tq.Offset = clampInt(queryInt(r, "page"), 0, 1000) * tq.Limit
	if !oneOf(tq.Filter, []string{"", "latest", "top", "unsolved", "solved", "events", "noreplies"}) {
		tq.Filter = ""
	}
	tq.Pins = tq.Filter == "" || tq.Filter == "latest"
	if tq.Filter == "latest" {
		tq.Filter = ""
	}
	if c := q.Get("category"); c != "" {
		cat, err := s.store.CategoryBySlug(ctx, a.f.ID, c)
		if err != nil {
			writeErr(w, http.StatusNotFound, "no such category")
			return
		}
		tq.CategoryID = cat.ID
	}
	topics, err := s.store.ListTopics(ctx, tq)
	if err != nil {
		s.fail(w, err, "topics")
		return
	}
	fs, cs := s.refs(ctx, 0, topics)
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics, "forums": fs, "categories": cs, "more": len(topics) == tq.Limit})
}

func (s *Server) publicTopic(w http.ResponseWriter, r *http.Request) (*access, *store.Topic, bool) {
	ctx := r.Context()
	t, err := s.store.GetTopic(ctx, idParam(r, "id"))
	if err != nil || t.State != "visible" {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	f, err := s.store.GetForum(ctx, t.ForumID)
	if err != nil || f.Visibility != "public" {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return &access{f: f}, t, true
}

func (s *Server) handlePublicTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.publicTopic(w, r)
	if !ok {
		return
	}
	s.store.AddView(ctx, t.ID)
	doc := s.topicDoc(ctx, a, t)
	doc["guest_posts"] = a.f.GuestPosts && !t.Closed && !t.Archived
	writeJSON(w, http.StatusOK, doc)
}

// guestAllowed is the rate limit for guests: five posts an hour per address.
func (s *Server) guestAllowed(r *http.Request) bool {
	ip := r.RemoteAddr
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	s.guestMu.Lock()
	defer s.guestMu.Unlock()
	cut := time.Now().Add(-time.Hour)
	keep := s.guestHits[ip][:0]
	for _, t := range s.guestHits[ip] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 5 {
		s.guestHits[ip] = keep
		return false
	}
	s.guestHits[ip] = append(keep, time.Now())
	return true
}

func (s *Server) handleGuestTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.publicForum(w, r)
	if !ok {
		return
	}
	if !a.f.GuestPosts || a.f.Archived {
		writeErr(w, http.StatusForbidden, "this forum does not take posts from guests")
		return
	}
	var in topicIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Website != "" {
		writeJSON(w, http.StatusCreated, map[string]any{"state": "pending"})
		return
	}
	if !s.guestAllowed(r) {
		writeErr(w, http.StatusTooManyRequests, "that is a lot of posts — try again in an hour")
		return
	}
	in.Kind, in.Poll = "", nil
	res, err := s.createTopic(ctx, a, in, true)
	if err != nil {
		s.userFail(w, err, "topic")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"state": res["state"]})
}

func (s *Server) handleGuestPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.publicTopic(w, r)
	if !ok {
		return
	}
	if !a.f.GuestPosts || a.f.Archived {
		writeErr(w, http.StatusForbidden, "this forum does not take posts from guests")
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Website != "" {
		writeJSON(w, http.StatusCreated, map[string]any{"state": "pending"})
		return
	}
	if !s.guestAllowed(r) {
		writeErr(w, http.StatusTooManyRequests, "that is a lot of posts — try again in an hour")
		return
	}
	in.Poll = nil
	p, err := s.createPost(ctx, a, t, in, true)
	if err != nil {
		s.userFail(w, err, "reply")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"state": p.State})
}

// ---- invitations without an account ----

func (s *Server) handlePublicInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	by := ""
	if u, err := s.store.GetUser(ctx, i.UserID); err == nil {
		by = s.actorName(u)
	}
	st := s.store.Stats(ctx, f.ID, time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339Nano))
	writeJSON(w, http.StatusOK, map[string]any{
		"forum": forumMini{f.ID, f.Slug, f.Name, f.Emoji, f.Color}, "about": f.About, "members": st.Members, "topics": st.Topics,
		"by": by, "note": i.Note, "email": i.Email, "role": i.Role, "why": inviteUsable(i), "accounts": i.Accounts && s.cfg.AuthMode == "local",
		"oidc": s.oidcEnabled() && s.cfg.AuthMode == "local", "oidc_name": s.oidcName(),
	})
}

// handleInviteSignup makes an account with an invitation and joins the forum.
func (s *Server) handleInviteSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	i, err := s.store.InviteByToken(ctx, chi.URLParam(r, "token"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invitation does not exist")
		return
	}
	if why := inviteUsable(i); why != "" {
		writeErr(w, http.StatusGone, why)
		return
	}
	if !i.Accounts || s.cfg.AuthMode != "local" {
		writeErr(w, http.StatusForbidden, "sign in with your account to use this invitation")
		return
	}
	f, err := s.store.GetForum(ctx, i.ForumID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invitation does not exist")
		return
	}
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = cleanText(in.Name, 80)
	if in.Name == "" || !strings.Contains(in.Email, "@") || len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "your name, your email and a password of at least 8 characters")
		return
	}
	if i.Email != "" && i.Email != in.Email {
		writeErr(w, http.StatusForbidden, "this invitation is for "+i.Email)
		return
	}
	if _, err := s.store.GetUserByEmail(ctx, in.Email); err == nil {
		writeErr(w, http.StatusConflict, "there is already an account with that email: sign in instead")
		return
	}
	h, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	u := &store.User{Email: in.Email, Name: in.Name, PasswordHash: string(h), Role: "member"}
	if err := s.store.CreateUser(ctx, u); err != nil {
		s.fail(w, err, "account")
		return
	}
	if err := s.join(ctx, f, u, i.Role, i.UserID); err != nil {
		s.fail(w, err, "join")
		return
	}
	s.store.UseInvite(ctx, i.ID)
	id, err := s.store.CreateSession(ctx, u.ID, s.cfg.SessionTTL)
	if err != nil {
		s.fail(w, err, "session")
		return
	}
	s.setSessionCookie(w, id, s.cfg.SessionTTL)
	s.log.Info("account made with an invitation", "forum", f.Slug, "email", u.Email)
	writeJSON(w, http.StatusOK, map[string]any{"slug": f.Slug, "user": u})
}

// ---- feeds ----

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"dc:creator"`
	Category    string `xml:"category,omitempty"`
	Description string `xml:"description"`
}

type rssDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	DC      string   `xml:"xmlns:dc,attr"`
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	} `xml:"channel"`
}

// handleFeed is a public forum's newest topics as RSS (?category=slug for one category).
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := strings.TrimSuffix(chi.URLParam(r, "forum"), ".rss")
	f, err := s.store.GetForumBySlug(ctx, slug)
	if err != nil || f.Visibility != "public" {
		http.NotFound(w, r)
		return
	}
	tq := store.TopicQuery{ForumIDs: []int64{f.ID}, Filter: "noreplies", Limit: 30}
	tq.Filter = ""
	var catName string
	if c := r.URL.Query().Get("category"); c != "" {
		if cat, err := s.store.CategoryBySlug(ctx, f.ID, c); err == nil {
			tq.CategoryID, catName = cat.ID, cat.Name
		}
	}
	topics, err := s.store.ListTopics(ctx, tq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var doc rssDoc
	doc.Version, doc.DC = "2.0", "http://purl.org/dc/elements/1.1/"
	doc.Channel.Title = f.Name
	if catName != "" {
		doc.Channel.Title += " — " + catName
	}
	doc.Channel.Link = s.link("/p/" + f.Slug)
	doc.Channel.Description = firstNonEmpty(f.About, f.Name)
	cats := map[int64]string{}
	if list, err := s.store.ListCategories(ctx, f.ID, 0); err == nil {
		for _, c := range list {
			cats[c.ID] = c.Name
		}
	}
	for _, t := range topics {
		first, err := s.store.PostByNumber(ctx, t.ID, 1)
		body := t.Excerpt
		if err == nil {
			body = first.Body
		}
		at, _ := time.Parse(time.RFC3339Nano, t.CreatedAt)
		link := s.link(fmt.Sprintf("/p/%s/t/%d", f.Slug, t.ID))
		doc.Channel.Items = append(doc.Channel.Items, rssItem{Title: t.Title, Link: link, GUID: link, PubDate: at.Format(time.RFC1123Z), Author: t.AuthorName,
			Category: cats[t.CategoryID], Description: mdHTML(body, s.cfg.BaseURL)})
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(doc)
}
