package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

const (
	maxBody  = 60000
	maxTitle = 200
)

var topicKinds = []string{"discussion", "question", "announcement", "event"}

// ---- lists ----

type forumMini struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
}

type catMini struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Emoji  string `json:"emoji"`
	Color  string `json:"color"`
	Parent int64  `json:"parent_id"`
	QA     bool   `json:"qa"`
}

// refs are the forums and categories a list of topics points at.
func (s *Server) refs(ctx context.Context, uid int64, topics []store.Topic) (map[int64]forumMini, map[int64]catMini) {
	fs, cs := map[int64]forumMini{}, map[int64]catMini{}
	for _, t := range topics {
		if _, ok := fs[t.ForumID]; !ok {
			if f, err := s.store.GetForum(ctx, t.ForumID); err == nil {
				fs[f.ID] = forumMini{f.ID, f.Slug, f.Name, f.Emoji, f.Color}
				cats, _ := s.store.ListCategories(ctx, f.ID, uid)
				for _, c := range cats {
					cs[c.ID] = catMini{c.ID, c.Name, c.Slug, c.Emoji, c.Color, c.ParentID, c.QA}
				}
			}
		}
	}
	return fs, cs
}

func periodSince(p string) string {
	days := map[string]int{"day": 1, "week": 7, "month": 30, "quarter": 91, "year": 365}[p]
	if days == 0 {
		return ""
	}
	return time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339Nano)
}

// handleTopics lists topics: one forum's (?forum=slug, optionally a
// category or tag) or, without a forum, from every forum the person is in.
func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	q := r.URL.Query()
	tq := store.TopicQuery{Viewer: u.ID, Filter: q.Get("filter"), Tag: q.Get("tag"), Kind: q.Get("kind"), Limit: clampInt(queryInt(r, "limit"), 0, 100), Since: periodSince(firstNonEmpty(q.Get("period"), "month")), NewSince: newSince()}
	if tq.Limit == 0 {
		tq.Limit = 30
	}
	tq.Offset = clampInt(queryInt(r, "page"), 0, 1000) * tq.Limit
	if ref := q.Get("forum"); ref != "" {
		a, err := s.forumAccess(ctx, u, ref)
		if err != nil || !a.canRead() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		tq.ForumIDs = []int64{a.f.ID}
		if c := q.Get("category"); c != "" {
			cat, err := s.store.CategoryBySlug(ctx, a.f.ID, c)
			if id, e := strconv.ParseInt(c, 10, 64); e == nil {
				cat, err = s.store.GetCategory(ctx, id)
			}
			if err != nil || cat.ForumID != a.f.ID {
				writeErr(w, http.StatusNotFound, "no such category")
				return
			}
			tq.CategoryID = cat.ID
		}
	} else {
		_, tq.ForumIDs, _ = s.readableForums(ctx, u)
	}
	if a := q.Get("author"); a != "" {
		tq.AuthorID, _ = strconv.ParseInt(a, 10, 64)
	}
	tq.Pins = tq.Filter == "" || tq.Filter == "latest"
	if tq.Filter == "latest" {
		tq.Filter = ""
	}
	topics, err := s.store.ListTopics(ctx, tq)
	if err != nil {
		s.fail(w, err, "topics")
		return
	}
	fs, cs := s.refs(ctx, u.ID, topics)
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics, "forums": fs, "categories": cs, "more": len(topics) == tq.Limit})
}

// handleEvents are the coming event topics of every forum the person is in.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	_, mine, _ := s.readableForums(ctx, u)
	topics, err := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: mine, Filter: "events", Limit: 50})
	if err != nil {
		s.fail(w, err, "events")
		return
	}
	fs, cs := s.refs(ctx, u.ID, topics)
	going := map[int64]string{}
	for _, t := range topics {
		rs, _ := s.store.RSVPs(ctx, t.ID)
		for _, x := range rs {
			if x.UserID == u.ID {
				going[t.ID] = x.Status
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics, "forums": fs, "categories": cs, "mine": going})
}

// ---- one topic ----

func (s *Server) topicDoc(ctx context.Context, a *access, t *store.Topic) map[string]any {
	posts, _ := s.store.ListPosts(ctx, t.ID, a.uid(), a.mod(), 0, 1000)
	for i := range posts {
		p := &posts[i]
		if p.State == "deleted" && !a.mod() && (a.u == nil || p.UserID != a.u.ID) {
			p.Body = ""
		}
		if poll, err := s.store.PollOfPost(ctx, p.ID, a.uid()); err == nil {
			p.Poll = poll
		}
	}
	var cat any
	if t.CategoryID > 0 {
		if c, err := s.store.GetCategory(ctx, t.CategoryID); err == nil {
			cat = catMini{c.ID, c.Name, c.Slug, c.Emoji, c.Color, c.ParentID, c.QA}
		}
	}
	canReply, why := a.canReply(t)
	doc := map[string]any{
		"topic": t, "posts": posts, "category": cat,
		"forum": map[string]any{"id": a.f.ID, "slug": a.f.Slug, "name": a.f.Name, "emoji": a.f.Emoji, "color": a.f.Color, "visibility": a.f.Visibility, "reactions": a.f.ReactionList(), "edit_minutes": a.f.EditMinutes},
		"can": map[string]any{"reply": canReply, "why": why, "moderate": a.mod(), "member": a.member(),
			"edit_topic": a.mod() || (a.u != nil && t.UserID == a.u.ID && a.member() && !t.Archived),
			"solve":      t.Kind == "question" && (a.mod() || (a.u != nil && t.UserID == a.u.ID && a.member())),
			"join":       a.can().Join},
	}
	if t.Kind == "event" {
		rs, _ := s.store.RSVPs(ctx, t.ID)
		doc["rsvps"] = rs
	}
	if a.f.Visibility == "public" && t.State == "visible" {
		doc["public_url"] = s.link(fmt.Sprintf("/p/%s/t/%d", a.f.Slug, t.ID))
	}
	return doc
}

func (s *Server) handleGetTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	if t.MovedTo > 0 && t.State == "deleted" {
		writeJSON(w, http.StatusOK, map[string]any{"moved_to": t.MovedTo})
		return
	}
	u := userFrom(ctx)
	if full, err := s.store.TopicFor(ctx, u.ID, t.ID); err == nil {
		t = full
	}
	if linkApp(ctx) == "" {
		if at := s.store.VisitedAt(ctx, t.ID, u.ID); at == "" || at < time.Now().Add(-30*time.Minute).UTC().Format(time.RFC3339Nano) {
			s.store.AddView(ctx, t.ID)
			t.Views++
		}
		if a.member() {
			s.store.TouchMember(ctx, a.f.ID, u.ID)
		}
	}
	writeJSON(w, http.StatusOK, s.topicDoc(ctx, a, t))
}

type pollIn struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Multi    bool     `json:"multi"`
	Public   bool     `json:"public"`
	Days     int      `json:"days"`
}

func (in *pollIn) clean() (*store.Poll, error) {
	if in == nil {
		return nil, nil
	}
	q := cleanText(in.Question, 300)
	opts := []string{}
	for _, o := range in.Options {
		if o = cleanText(o, 200); o != "" && len(opts) < 20 && !oneOf(o, opts) {
			opts = append(opts, o)
		}
	}
	if q == "" && len(opts) == 0 {
		return nil, nil
	}
	if q == "" || len(opts) < 2 {
		return nil, &userErr{"a poll needs a question and at least two choices"}
	}
	p := &store.Poll{Question: q, Options: opts, Multi: in.Multi, Public: in.Public}
	if in.Days > 0 {
		at := time.Now().AddDate(0, 0, clampInt(in.Days, 1, 365)).UTC().Format(time.RFC3339Nano)
		p.ClosesAt = &at
	}
	return p, nil
}

type topicIn struct {
	Forum      string   `json:"forum"`
	CategoryID int64    `json:"category_id"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Kind       string   `json:"kind"`
	Tags       []string `json:"tags"`
	EventStart string   `json:"event_start"`
	EventEnd   string   `json:"event_end"`
	EventPlace string   `json:"event_place"`
	Poll       *pollIn  `json:"poll"`
	Pinned     int      `json:"pinned"`
	Closed     bool     `json:"closed"`
	Unlisted   bool     `json:"unlisted"`
	DraftKey   string   `json:"draft_key"`
	SourceRef  string   `json:"source_ref"`
	GuestName  string   `json:"guest_name"`
	GuestEmail string   `json:"guest_email"`
	Website    string   `json:"website"` // the honeypot: people leave it empty
	IdemKey    string   `json:"-"`
}

var reEventTime = func(v string) bool {
	if v == "" {
		return true
	}
	for _, f := range []string{"2006-01-02T15:04", "2006-01-02"} {
		if _, err := time.Parse(f, v); err == nil {
			return true
		}
	}
	return false
}

// checkPost runs a forum's rules over a new post: banned words refuse it,
// held words, a new member's first posts and guests hold it for a
// moderator. Slow mode keeps a person from posting too often.
func (s *Server) checkPost(ctx context.Context, a *access, t *store.Topic, text string, guest bool) (state string, err error) {
	if w := containsWord(text, a.f.BlockedWords); w != "" && !a.mod() {
		return "", &userErr{"your post has a word this forum does not allow (“" + w + "”)"}
	}
	if !a.mod() && !guest {
		wait := a.f.SlowMode
		var since string
		if t != nil && t.SlowMode > 0 {
			wait = t.SlowMode
			since = s.store.LastPostAt(ctx, a.f.ID, a.uid(), t.ID)
		} else if wait > 0 {
			since = s.store.LastPostAt(ctx, a.f.ID, a.uid(), 0)
		}
		if wait > 0 && since != "" {
			if at, e := time.Parse(time.RFC3339Nano, since); e == nil {
				if left := time.Duration(wait)*time.Second - time.Since(at); left > 0 {
					return "", &userErr{"slow mode is on: you can post again in " + waitWords(left)}
				}
			}
		}
	}
	switch {
	case guest:
		return "pending", nil
	case a.mod():
		return "visible", nil
	case containsWord(text, a.f.HeldWords) != "":
		return "pending", nil
	case a.f.ApproveFirst > 0 && s.store.CountPostsBy(ctx, a.f.ID, a.uid()) < a.f.ApproveFirst:
		return "pending", nil
	}
	return "visible", nil
}

func waitWords(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds())+1)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes())+1)
	}
	return fmt.Sprintf("%d hours", int(d.Hours())+1)
}

func cleanBody(v string) (string, error) {
	b := strings.TrimRight(strings.ReplaceAll(v, "\r\n", "\n"), " \n\t")
	b = strings.TrimLeft(b, "\n")
	if utf8.RuneCountInString(strings.TrimSpace(b)) < 2 {
		return "", &userErr{"write a little more"}
	}
	if utf8.RuneCountInString(b) > maxBody {
		return "", &userErr{fmt.Sprintf("a post can be up to %d characters", maxBody)}
	}
	return b, nil
}

func (s *Server) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in topicIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a, err := s.forumAccess(ctx, u, in.Forum)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such forum")
		return
	}
	res, err := s.createTopic(ctx, a, in, false)
	if err != nil {
		s.userFail(w, err, "create topic")
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// createTopic is starting a topic, for members and (guest = true) for
// people without an account on a public forum.
func (s *Server) createTopic(ctx context.Context, a *access, in topicIn, guest bool) (map[string]any, error) {
	var cat *store.Category
	if in.CategoryID > 0 {
		c, err := s.store.GetCategory(ctx, in.CategoryID)
		if err != nil || c.ForumID != a.f.ID {
			return nil, &userErr{"pick a category of this forum"}
		}
		cat = c
	}
	if !guest {
		if ok, why := a.canStart(cat); !ok {
			return nil, &userErr{why}
		}
	} else if cat != nil && cat.Starts == "mods" {
		return nil, &userErr{"only moderators start topics in " + cat.Name}
	}
	title := cleanText(strings.Join(strings.Fields(in.Title), " "), maxTitle)
	if utf8.RuneCountInString(title) < 3 {
		return nil, &userErr{"give the topic a title"}
	}
	body, err := cleanBody(in.Body)
	if err != nil {
		return nil, err
	}
	kind := firstNonEmpty(in.Kind, "discussion")
	if cat != nil && cat.QA && in.Kind == "" {
		kind = "question"
	}
	if !oneOf(kind, topicKinds) {
		return nil, &userErr{"a topic is a discussion, a question, an announcement or an event"}
	}
	if kind == "announcement" && !a.mod() {
		kind = "discussion"
	}
	if kind == "event" {
		if in.EventStart == "" || !reEventTime(in.EventStart) || !reEventTime(in.EventEnd) {
			return nil, &userErr{"an event needs its date (and time)"}
		}
	}
	poll, err := in.Poll.clean()
	if err != nil {
		return nil, err
	}
	state, err := s.checkPost(ctx, a, nil, title+"\n"+body, guest)
	if err != nil {
		return nil, err
	}
	src := sourceOf(ctx)
	if src != "" && in.SourceRef != "" {
		if old, err := s.store.TopicBySource(ctx, a.f.ID, src, clip(in.SourceRef, 200)); err == nil {
			return map[string]any{"topic": old, "state": old.State, "url": s.link(fmt.Sprintf("/t/%d", old.ID)), "existing": true}, nil
		}
	}
	t := &store.Topic{ForumID: a.f.ID, CategoryID: in.CategoryID, Title: title, Kind: kind, State: state, Tags: in.Tags, Source: src, SourceRef: clip(in.SourceRef, 200)}
	if kind == "event" {
		t.EventStart, t.EventEnd, t.EventPlace = in.EventStart, in.EventEnd, cleanText(in.EventPlace, 200)
	}
	if a.mod() {
		t.Pinned, t.Closed, t.Unlisted = clampInt(in.Pinned, 0, 2), in.Closed, in.Unlisted
	}
	p := &store.Post{Body: body, State: state, Source: src}
	if guest {
		name := cleanText(in.GuestName, 60)
		if name == "" {
			return nil, &userErr{"tell us your name"}
		}
		t.AuthorName, p.AuthorName = name+" (guest)", name+" (guest)"
		p.GuestEmail = cleanText(in.GuestEmail, 200)
	} else {
		t.UserID, p.UserID = a.uid(), a.uid()
		t.AuthorName, p.AuthorName = s.actorName(a.u), s.actorName(a.u)
	}
	if err := s.store.CreateTopic(ctx, t, p); err != nil {
		return nil, err
	}
	s.store.MarkUploadsUsed(ctx, uploadIDs(body))
	if poll != nil {
		poll.PostID, poll.TopicID, poll.ForumID, poll.UserID = p.ID, t.ID, a.f.ID, p.UserID
		if err := s.store.SavePoll(ctx, poll); err != nil {
			s.log.Warn("poll", "err", err)
		}
	}
	if !guest {
		st := s.settingsOf(ctx, a.uid())
		if st.AutoWatch {
			_ = s.store.SetTopicLevel(ctx, t.ID, a.uid(), "watching")
		}
		s.store.MarkRead(ctx, t.ID, a.uid(), 1)
		if in.DraftKey != "" {
			s.store.DeleteDraft(ctx, a.uid(), in.DraftKey)
		}
	}
	s.afterPost(ctx, a, t, p, true)
	return map[string]any{"topic": t, "post": p, "state": state, "url": s.link(fmt.Sprintf("/t/%d", t.ID))}, nil
}

// afterPost tells everyone what a new post needs: the live pages, the
// people it concerns, the moderators when it waits, the Core bus.
func (s *Server) afterPost(ctx context.Context, a *access, t *store.Topic, p *store.Post, isTopic bool) {
	ctx = context.WithoutCancel(ctx)
	if p.State == "pending" {
		s.tellMods(ctx, a.f, store.Notification{Kind: "pending", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: p.UserID, ActorName: p.AuthorName, Title: t.Title, Excerpt: excerpt(p.Body, 200)}, p.UserID)
		return
	}
	if p.State != "visible" {
		return
	}
	kind := "post"
	if isTopic {
		kind = "topic"
	}
	s.live(a.f.ID, map[string]any{"type": kind, "topic_id": t.ID, "post_id": p.ID, "number": p.Number, "user_id": p.UserID, "category_id": t.CategoryID})
	go s.notifyPost(ctx, a, t, p, isTopic)
	url := s.link(fmt.Sprintf("/t/%d/%d", t.ID, p.Number))
	if p.UserID > 0 {
		if isTopic {
			s.publish(ctx, p.UserID, "forum.topic_created", t.Title, map[string]any{"forum": a.f.Slug, "forum_name": a.f.Name, "topic_id": t.ID, "kind": t.Kind, "url": url})
		} else {
			s.publish(ctx, p.UserID, "forum.post_created", t.Title, map[string]any{"forum": a.f.Slug, "topic_id": t.ID, "post_id": p.ID, "number": p.Number, "url": url})
		}
	}
}

type topicPatch struct {
	Title      *string   `json:"title"`
	CategoryID *int64    `json:"category_id"`
	Tags       *[]string `json:"tags"`
	Kind       *string   `json:"kind"`
	EventStart *string   `json:"event_start"`
	EventEnd   *string   `json:"event_end"`
	EventPlace *string   `json:"event_place"`
}

func (s *Server) handleUpdateTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	first, err := s.store.PostByNumber(ctx, t.ID, 1)
	if err != nil {
		s.fail(w, err, "topic")
		return
	}
	if !a.mod() && (a.u == nil || t.UserID != a.u.ID || !a.canEditPost(first, t)) {
		writeErr(w, http.StatusForbidden, "you cannot change this topic")
		return
	}
	if !mayChange(ctx, t.Source) {
		writeErr(w, http.StatusForbidden, "this app can only change the topics it started")
		return
	}
	var in topicPatch
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldTitle := t.Title
	if in.Title != nil {
		tt := cleanText(strings.Join(strings.Fields(*in.Title), " "), maxTitle)
		if utf8.RuneCountInString(tt) < 3 {
			writeErr(w, http.StatusBadRequest, "give the topic a title")
			return
		}
		t.Title = tt
	}
	if in.CategoryID != nil && *in.CategoryID != t.CategoryID {
		if *in.CategoryID > 0 {
			c, err := s.store.GetCategory(ctx, *in.CategoryID)
			if err != nil || c.ForumID != a.f.ID {
				writeErr(w, http.StatusBadRequest, "pick a category of this forum")
				return
			}
			if ok, why := a.canStart(c); !ok {
				writeErr(w, http.StatusForbidden, why)
				return
			}
		}
		t.CategoryID = *in.CategoryID
	}
	if in.Tags != nil {
		t.Tags = *in.Tags
	}
	if in.Kind != nil {
		if !oneOf(*in.Kind, topicKinds) || (*in.Kind == "announcement" && !a.mod()) {
			writeErr(w, http.StatusBadRequest, "a discussion, a question or an event")
			return
		}
		t.Kind = *in.Kind
		if t.Kind != "question" {
			t.SolvedPostID = 0
		}
	}
	if in.EventStart != nil {
		t.EventStart = strings.TrimSpace(*in.EventStart)
	}
	if in.EventEnd != nil {
		t.EventEnd = strings.TrimSpace(*in.EventEnd)
	}
	if in.EventPlace != nil {
		t.EventPlace = cleanText(*in.EventPlace, 200)
	}
	if t.Kind == "event" && (t.EventStart == "" || !reEventTime(t.EventStart) || !reEventTime(t.EventEnd)) {
		writeErr(w, http.StatusBadRequest, "an event needs its date (and time)")
		return
	}
	if err := s.store.UpdateTopic(ctx, t); err != nil {
		s.fail(w, err, "save topic")
		return
	}
	if oldTitle != t.Title {
		_ = s.store.EditPost(ctx, first, first.Body, oldTitle, a.uid(), s.actorName(a.u), "title")
		if a.mod() && a.u != nil && t.UserID != a.u.ID {
			s.modLog(ctx, a, "renamed a topic", "topic", t.ID, oldTitle+" → "+t.Title)
		}
	}
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": t.ID})
	t, _ = s.store.TopicFor(ctx, a.uid(), t.ID)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	mine := a.u != nil && t.UserID == a.u.ID
	if !a.mod() && !(mine && t.PostsN <= 1) {
		writeErr(w, http.StatusForbidden, "only a moderator can remove a topic with replies")
		return
	}
	if !mayChange(ctx, t.Source) {
		writeErr(w, http.StatusForbidden, "this app can only remove the topics it started")
		return
	}
	t.State = "deleted"
	t.Pinned = 0
	if err := s.store.UpdateTopic(ctx, t); err != nil {
		s.fail(w, err, "delete")
		return
	}
	if t.UserID > 0 {
		s.store.RecountMember(ctx, a.f.ID, t.UserID)
	}
	if a.mod() && !mine {
		s.modLog(ctx, a, "removed a topic", "topic", t.ID, t.Title)
	}
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": t.ID})
	w.WriteHeader(http.StatusNoContent)
}

// handleTopicStatus is a moderator closing, pinning, archiving, listing or hiding a topic.
func (s *Server) handleTopicStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Closed      *bool   `json:"closed"`
		Archived    *bool   `json:"archived"`
		Pinned      *int    `json:"pinned"`
		PinnedDays  *int    `json:"pinned_days"`
		Unlisted    *bool   `json:"unlisted"`
		CloseInDays *int    `json:"close_in_days"`
		SlowMode    *int    `json:"slow_mode"`
		State       *string `json:"state"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// the author of a question may close it; everything else is for moderators
	if !a.mod() {
		if a.u == nil || t.UserID != a.u.ID || in.Closed == nil || in.Archived != nil || in.Pinned != nil || in.Unlisted != nil || in.State != nil || in.SlowMode != nil || in.CloseInDays != nil {
			writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
			return
		}
	}
	var did []string
	if in.Closed != nil && *in.Closed != t.Closed {
		t.Closed = *in.Closed
		did = append(did, map[bool]string{true: "closed", false: "opened"}[t.Closed])
	}
	if in.Archived != nil && *in.Archived != t.Archived {
		t.Archived = *in.Archived
		did = append(did, map[bool]string{true: "archived", false: "unarchived"}[t.Archived])
	}
	if in.Pinned != nil {
		t.Pinned = clampInt(*in.Pinned, 0, 2)
		t.PinnedUntil = nil
		if t.Pinned > 0 && in.PinnedDays != nil && *in.PinnedDays > 0 {
			at := time.Now().AddDate(0, 0, *in.PinnedDays).UTC().Format(time.RFC3339Nano)
			t.PinnedUntil = &at
		}
		did = append(did, map[int]string{0: "unpinned", 1: "pinned in its category", 2: "pinned on the front page"}[t.Pinned])
	}
	if in.Unlisted != nil && *in.Unlisted != t.Unlisted {
		t.Unlisted = *in.Unlisted
		did = append(did, map[bool]string{true: "unlisted", false: "listed"}[t.Unlisted])
	}
	if in.CloseInDays != nil {
		t.CloseAt = nil
		if *in.CloseInDays > 0 {
			at := time.Now().AddDate(0, 0, *in.CloseInDays).UTC().Format(time.RFC3339Nano)
			t.CloseAt = &at
			did = append(did, fmt.Sprintf("set to close in %d days", *in.CloseInDays))
		}
	}
	if in.SlowMode != nil {
		t.SlowMode = clampInt(*in.SlowMode, 0, 86400)
		did = append(did, "set slow mode")
	}
	if in.State != nil {
		if !oneOf(*in.State, []string{"visible", "hidden"}) {
			writeErr(w, http.StatusBadRequest, "visible or hidden")
			return
		}
		if t.State != *in.State {
			did = append(did, map[string]string{"visible": "showed", "hidden": "hid"}[*in.State])
		}
		t.State = *in.State
	}
	if err := s.store.UpdateTopic(ctx, t); err != nil {
		s.fail(w, err, "save")
		return
	}
	if len(did) > 0 && a.mod() {
		s.modLog(ctx, a, strings.Join(did, ", "), "topic", t.ID, t.Title)
	}
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": t.ID})
	t, _ = s.store.TopicFor(ctx, a.uid(), t.ID)
	writeJSON(w, http.StatusOK, t)
}

// handleMergeTopic moves every post of a topic to the end of another one.
func (s *Server) handleMergeTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	var in struct {
		Into int64 `json:"into"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	into, err := s.store.GetTopic(ctx, in.Into)
	if err != nil || into.ForumID != a.f.ID || into.ID == t.ID || into.State == "deleted" {
		writeErr(w, http.StatusBadRequest, "pick another topic of this forum")
		return
	}
	posts, _ := s.store.ListPosts(ctx, t.ID, 0, true, 0, 10000)
	ids := make([]int64, 0, len(posts))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	if err := s.store.MovePosts(ctx, ids, into.ID); err != nil {
		s.fail(w, err, "merge")
		return
	}
	t.State, t.MovedTo, t.Pinned = "deleted", into.ID, 0
	_ = s.store.UpdateTopic(ctx, t)
	s.modLog(ctx, a, "merged a topic into “"+into.Title+"”", "topic", t.ID, t.Title)
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": into.ID})
	writeJSON(w, http.StatusOK, map[string]any{"topic_id": into.ID})
}

// handleSplitTopic moves some posts into a new topic of their own.
func (s *Server) handleSplitTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	if !a.mod() {
		writeErr(w, http.StatusForbidden, "only the forum's moderators can do that")
		return
	}
	var in struct {
		PostIDs    []int64 `json:"post_ids"`
		Title      string  `json:"title"`
		CategoryID int64   `json:"category_id"`
		Into       int64   `json:"into"`
	}
	if err := decode(r, &in); err != nil || len(in.PostIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "pick the posts to move")
		return
	}
	for _, id := range in.PostIDs {
		p, err := s.store.GetPost(ctx, id)
		if err != nil || p.TopicID != t.ID {
			writeErr(w, http.StatusBadRequest, "every post has to come from this topic")
			return
		}
		if p.Number == 1 {
			writeErr(w, http.StatusBadRequest, "the first post stays with its topic")
			return
		}
	}
	var dest *store.Topic
	if in.Into > 0 {
		d, err := s.store.GetTopic(ctx, in.Into)
		if err != nil || d.ForumID != a.f.ID || d.ID == t.ID {
			writeErr(w, http.StatusBadRequest, "pick another topic of this forum")
			return
		}
		dest = d
	} else {
		title := cleanText(in.Title, maxTitle)
		if utf8.RuneCountInString(title) < 3 {
			writeErr(w, http.StatusBadRequest, "give the new topic a title")
			return
		}
		cid := in.CategoryID
		if cid == 0 {
			cid = t.CategoryID
		}
		if cid > 0 {
			if c, err := s.store.GetCategory(ctx, cid); err != nil || c.ForumID != a.f.ID {
				writeErr(w, http.StatusBadRequest, "pick a category of this forum")
				return
			}
		}
		first, err := s.store.GetPost(ctx, in.PostIDs[0])
		if err != nil {
			s.fail(w, err, "split")
			return
		}
		// the new topic starts empty and takes the posts
		dest = &store.Topic{ForumID: a.f.ID, CategoryID: cid, UserID: first.UserID, AuthorName: first.AuthorName, Title: title, Kind: "discussion", State: "visible", CreatedAt: first.CreatedAt}
		placeholder := &store.Post{UserID: first.UserID, AuthorName: first.AuthorName, Body: first.Body, State: "deleted", CreatedAt: first.CreatedAt}
		if err := s.store.CreateTopic(ctx, dest, placeholder); err != nil {
			s.fail(w, err, "split")
			return
		}
		// drop the placeholder: the moved posts are numbered from 1
		_, _ = s.store.DB.ExecContext(ctx, `DELETE FROM post_search WHERE rowid = ?`, placeholder.ID)
		_, _ = s.store.DB.ExecContext(ctx, `DELETE FROM posts WHERE id = ?`, placeholder.ID)
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE topics SET highest = 0 WHERE id = ?`, dest.ID)
	}
	if err := s.store.MovePosts(ctx, in.PostIDs, dest.ID); err != nil {
		s.fail(w, err, "split")
		return
	}
	s.modLog(ctx, a, fmt.Sprintf("moved %d %s to “%s”", len(in.PostIDs), plural(len(in.PostIDs), "post", "posts"), dest.Title), "topic", t.ID, t.Title)
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": t.ID})
	writeJSON(w, http.StatusOK, map[string]any{"topic_id": dest.ID})
}

func (s *Server) handleReadTopic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Number int `json:"number"`
	}
	_ = decode(r, &in)
	n := in.Number
	if n <= 0 || n > t.Highest {
		n = t.Highest
	}
	if a.u != nil {
		s.store.MarkRead(ctx, t.ID, a.u.ID, n)
		s.store.ReadTopicNotifications(ctx, a.u.ID, t.ID, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"last_read": n, "unread_notifications": s.store.UnreadNotifications(ctx, a.uid())})
}

func (s *Server) handleTopicLevel(w http.ResponseWriter, r *http.Request) {
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Level string `json:"level"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Level, levels) {
		writeErr(w, http.StatusBadRequest, "watching, tracking, normal or muted")
		return
	}
	if err := s.store.SetTopicLevel(r.Context(), t.ID, a.uid(), in.Level); err != nil {
		s.fail(w, err, "level")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"level": in.Level})
}

// handleSolve marks the reply that answered a question (post 0 = none).
func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	if t.Kind != "question" {
		writeErr(w, http.StatusBadRequest, "only a question gets a solution")
		return
	}
	if !a.mod() && !(a.u != nil && t.UserID == a.u.ID && a.member()) {
		writeErr(w, http.StatusForbidden, "the person who asked (or a moderator) picks the solution")
		return
	}
	var in struct {
		PostID int64 `json:"post_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	prev := t.SolvedPostID
	var p *store.Post
	if in.PostID > 0 {
		var err error
		p, err = s.store.GetPost(ctx, in.PostID)
		if err != nil || p.TopicID != t.ID || p.State != "visible" || p.Number == 1 {
			writeErr(w, http.StatusBadRequest, "pick a reply of this topic")
			return
		}
	}
	t.SolvedPostID = in.PostID
	if err := s.store.UpdateTopic(ctx, t); err != nil {
		s.fail(w, err, "solve")
		return
	}
	for _, id := range []int64{prev, in.PostID} {
		if id > 0 {
			if q, err := s.store.GetPost(ctx, id); err == nil && q.UserID > 0 {
				s.store.RecountMember(ctx, a.f.ID, q.UserID)
			}
		}
	}
	if p != nil && p.UserID > 0 && p.UserID != a.uid() {
		s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "solved", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: t.Title})
	}
	if p != nil {
		s.publish(ctx, a.uid(), "forum.solved", t.Title, map[string]any{"forum": a.f.Slug, "topic_id": t.ID, "post_id": p.ID, "url": s.link(fmt.Sprintf("/t/%d/%d", t.ID, p.Number))})
	}
	s.live(a.f.ID, map[string]any{"type": "topic", "topic_id": t.ID})
	t, _ = s.store.TopicFor(ctx, a.uid(), t.ID)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleRSVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	if t.Kind != "event" {
		writeErr(w, http.StatusBadRequest, "only an event takes answers")
		return
	}
	if !a.member() {
		writeErr(w, http.StatusForbidden, "join the forum to answer")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Status, []string{"", "going", "maybe", "no"}) {
		writeErr(w, http.StatusBadRequest, "going, maybe or no")
		return
	}
	if err := s.store.SetRSVP(ctx, t.ID, a.uid(), in.Status); err != nil {
		s.fail(w, err, "answer")
		return
	}
	rs, _ := s.store.RSVPs(ctx, t.ID)
	s.live(a.f.ID, map[string]any{"type": "rsvp", "topic_id": t.ID})
	writeJSON(w, http.StatusOK, map[string]any{"rsvps": rs})
}

// handleSimilar finds topics like the one being written.
func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var forums []int64
	if ref := r.URL.Query().Get("forum"); ref != "" {
		a, err := s.forumAccess(ctx, u, ref)
		if err != nil || !a.canRead() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		forums = []int64{a.f.ID}
	} else {
		forums, _, _ = s.readableForums(ctx, u)
	}
	hits, err := s.store.Similar(ctx, forums, r.URL.Query().Get("title"), 6)
	if err != nil {
		s.fail(w, err, "similar")
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

// ---- posts ----

type postIn struct {
	Body       string  `json:"body"`
	ReplyTo    int     `json:"reply_to"`
	DraftKey   string  `json:"draft_key"`
	Reason     string  `json:"reason"`
	Poll       *pollIn `json:"poll"`
	SourceRef  string  `json:"source_ref"`
	GuestName  string  `json:"guest_name"`
	GuestEmail string  `json:"guest_email"`
	Website    string  `json:"website"`
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, ok := s.topicParam(w, r)
	if !ok {
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p, err := s.createPost(ctx, a, t, in, false)
	if err != nil {
		s.userFail(w, err, "reply")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"post": p, "state": p.State, "url": s.link(fmt.Sprintf("/t/%d/%d", t.ID, p.Number))})
}

func (s *Server) createPost(ctx context.Context, a *access, t *store.Topic, in postIn, guest bool) (*store.Post, error) {
	if !guest {
		if ok, why := a.canReply(t); !ok {
			return nil, &userErr{why}
		}
	} else if t.Closed || t.Archived {
		return nil, &userErr{"this topic is closed"}
	}
	if t.State != "visible" && !a.mod() {
		return nil, &userErr{"this topic is waiting for a moderator"}
	}
	body, err := cleanBody(in.Body)
	if err != nil {
		return nil, err
	}
	if in.ReplyTo > 0 {
		if q, err := s.store.PostByNumber(ctx, t.ID, in.ReplyTo); err != nil || q.State != "visible" {
			in.ReplyTo = 0
		}
	}
	poll, err := in.Poll.clean()
	if err != nil {
		return nil, err
	}
	state, err := s.checkPost(ctx, a, t, body, guest)
	if err != nil {
		return nil, err
	}
	p := &store.Post{TopicID: t.ID, ForumID: a.f.ID, ReplyTo: in.ReplyTo, Body: body, State: state, Source: sourceOf(ctx), SourceRef: clip(in.SourceRef, 200)}
	if guest {
		name := cleanText(in.GuestName, 60)
		if name == "" {
			return nil, &userErr{"tell us your name"}
		}
		p.AuthorName, p.GuestEmail = name+" (guest)", cleanText(in.GuestEmail, 200)
	} else {
		p.UserID, p.AuthorName = a.uid(), s.actorName(a.u)
	}
	if err := s.store.CreatePost(ctx, p); err != nil {
		return nil, err
	}
	s.store.MarkUploadsUsed(ctx, uploadIDs(body))
	if poll != nil {
		poll.PostID, poll.TopicID, poll.ForumID, poll.UserID = p.ID, t.ID, a.f.ID, p.UserID
		_ = s.store.SavePoll(ctx, poll)
	}
	if !guest {
		if s.settingsOf(ctx, a.uid()).AutoWatch {
			s.store.RaiseTopicLevel(ctx, t.ID, a.uid(), "tracking")
		}
		s.store.MarkRead(ctx, t.ID, a.uid(), p.Number)
		if in.DraftKey != "" {
			s.store.DeleteDraft(ctx, a.uid(), in.DraftKey)
		}
	}
	if full, err := s.store.GetPost(ctx, p.ID); err == nil {
		p = full
	}
	s.afterPost(ctx, a, t, p, false)
	return p, nil
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if p.State == "deleted" && !a.mod() && (a.u == nil || p.UserID != a.u.ID) {
		p.Body = ""
	}
	if poll, err := s.store.PollOfPost(r.Context(), p.ID, a.uid()); err == nil {
		p.Poll = poll
	}
	writeJSON(w, http.StatusOK, map[string]any{"post": p, "topic": map[string]any{"id": t.ID, "title": t.Title, "forum": a.f.Slug}, "can_edit": a.canEditPost(p, t)})
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.canEditPost(p, t) {
		writeErr(w, http.StatusForbidden, "you cannot change this post (any more)")
		return
	}
	if !mayChange(ctx, p.Source) {
		writeErr(w, http.StatusForbidden, "this app can only change the posts it wrote")
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	body, err := cleanBody(in.Body)
	if err != nil {
		s.userFail(w, err, "edit")
		return
	}
	if w2 := containsWord(body, a.f.BlockedWords); w2 != "" && !a.mod() {
		writeErr(w, http.StatusBadRequest, "your post has a word this forum does not allow (“"+w2+"”)")
		return
	}
	if body == p.Body {
		writeJSON(w, http.StatusOK, p)
		return
	}
	old := p.Body
	p.Body = body
	if err := s.store.EditPost(ctx, p, old, "", a.uid(), s.actorName(a.u), cleanText(in.Reason, 200)); err != nil {
		s.fail(w, err, "edit")
		return
	}
	s.store.MarkUploadsUsed(ctx, uploadIDs(body))
	if in.Poll != nil {
		if poll, err := in.Poll.clean(); err == nil && poll != nil {
			poll.PostID, poll.TopicID, poll.ForumID, poll.UserID = p.ID, t.ID, a.f.ID, p.UserID
			_ = s.store.SavePoll(ctx, poll)
		}
	}
	if a.mod() && p.UserID != a.uid() && !p.Wiki {
		s.modLog(ctx, a, "edited a post", "post", p.ID, t.Title)
	}
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	p, _ = s.store.GetPost(ctx, p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	mine := a.u != nil && p.UserID == a.u.ID
	if !a.mod() && !mine {
		writeErr(w, http.StatusForbidden, "you cannot remove this post")
		return
	}
	if !mayChange(ctx, p.Source) {
		writeErr(w, http.StatusForbidden, "this app can only remove the posts it wrote")
		return
	}
	if p.Number == 1 {
		if !a.mod() && t.PostsN > 1 {
			writeErr(w, http.StatusBadRequest, "the first post of a topic with replies stays; ask a moderator")
			return
		}
		t.State, t.Pinned = "deleted", 0
		_ = s.store.UpdateTopic(ctx, t)
	}
	if err := s.store.SetPostState(ctx, p, "deleted", ""); err != nil {
		s.fail(w, err, "delete")
		return
	}
	if t.SolvedPostID == p.ID {
		t.SolvedPostID = 0
		_ = s.store.UpdateTopic(ctx, t)
	}
	if a.mod() && !mine {
		s.modLog(ctx, a, "removed a post", "post", p.ID, t.Title+" #"+strconv.Itoa(p.Number))
	}
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestorePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	mine := a.u != nil && p.UserID == a.u.ID
	if p.State != "deleted" || (!a.mod() && !mine) {
		writeErr(w, http.StatusForbidden, "you cannot bring this post back")
		return
	}
	if err := s.store.SetPostState(ctx, p, "visible", ""); err != nil {
		s.fail(w, err, "restore")
		return
	}
	if p.Number == 1 && t.State == "deleted" {
		t.State = "visible"
		_ = s.store.UpdateTopic(ctx, t)
	}
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleRevisions(w http.ResponseWriter, r *http.Request) {
	a, _, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if p.State == "deleted" && !a.mod() && (a.u == nil || p.UserID != a.u.ID) {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	list, err := s.store.Revisions(r.Context(), p.ID)
	if err != nil {
		s.fail(w, err, "history")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": p.Body, "revisions": list})
}

func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.member() || p.State != "visible" {
		writeErr(w, http.StatusForbidden, "join the forum to react")
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Emoji, a.f.ReactionList()) {
		writeErr(w, http.StatusBadRequest, "pick one of the forum's reactions")
		return
	}
	if p.UserID == a.uid() {
		writeErr(w, http.StatusBadRequest, "you cannot react to your own post")
		return
	}
	added, err := s.store.ToggleReaction(ctx, p, a.uid(), in.Emoji)
	if err != nil {
		s.fail(w, err, "react")
		return
	}
	if added && p.UserID > 0 {
		s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "reaction", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: t.Title, Excerpt: in.Emoji + " " + excerpt(p.Body, 120)})
	}
	mine := []string{}
	for _, x := range func() []store.Reactor { l, _ := s.store.Reactors(ctx, p.ID); return l }() {
		if x.UserID == a.uid() {
			mine = append(mine, x.Emoji)
		}
	}
	s.live(a.f.ID, map[string]any{"type": "reaction", "topic_id": t.ID, "post_id": p.ID})
	writeJSON(w, http.StatusOK, map[string]any{"reactions": p.Reactions, "likes_n": p.LikesN, "mine": mine})
}

func (s *Server) handleReactors(w http.ResponseWriter, r *http.Request) {
	_, _, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	list, err := s.store.Reactors(r.Context(), p.ID)
	if err != nil {
		s.fail(w, err, "reactions")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleFlag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.member() {
		writeErr(w, http.StatusForbidden, "join the forum to flag")
		return
	}
	var in struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Reason, []string{"spam", "rude", "off_topic", "other"}) {
		writeErr(w, http.StatusBadRequest, "spam, rude, off topic or something else")
		return
	}
	if in.Reason == "other" && strings.TrimSpace(in.Note) == "" {
		writeErr(w, http.StatusBadRequest, "tell the moderators what is wrong")
		return
	}
	f := &store.Flag{ForumID: a.f.ID, PostID: p.ID, UserID: a.uid(), Reason: in.Reason, Note: cleanText(in.Note, 500)}
	if err := s.store.AddFlag(ctx, f); err != nil {
		s.fail(w, err, "flag")
		return
	}
	s.tellMods(ctx, a.f, store.Notification{Kind: "flag", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u), Title: t.Title, Excerpt: flagWords[in.Reason] + ": " + excerpt(p.Body, 140)}, a.uid())
	s.publish(ctx, a.uid(), "forum.flagged", t.Title, map[string]any{"forum": a.f.Slug, "topic_id": t.ID, "post_id": p.ID, "reason": in.Reason})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Thanks — the moderators will have a look"})
}

var flagWords = map[string]string{"spam": "Spam", "rude": "Rude or unkind", "off_topic": "Off topic", "other": "Something else"}

func (s *Server) handleBookmark(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, _, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if a.u == nil {
		writeErr(w, http.StatusUnauthorized, "sign in")
		return
	}
	if r.Method == http.MethodDelete {
		_ = s.store.DeleteBookmark(ctx, a.u.ID, p.ID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var in struct {
		Note     string `json:"note"`
		RemindAt string `json:"remind_at"` // RFC 3339 or "" for none
	}
	_ = decode(r, &in)
	b := &store.Bookmark{PostID: p.ID, Note: cleanText(in.Note, 300)}
	if in.RemindAt != "" {
		at, err := time.Parse(time.RFC3339, in.RemindAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the reminder needs a date and time")
			return
		}
		v := at.UTC().Format(time.RFC3339Nano)
		b.RemindAt = &v
	}
	if err := s.store.SaveBookmark(ctx, a.u.ID, b); err != nil {
		s.fail(w, err, "bookmark")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleWiki(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, t, p, ok := s.postParam(w, r)
	if !ok {
		return
	}
	if !a.mod() && !(a.u != nil && p.UserID == a.u.ID && a.member()) {
		writeErr(w, http.StatusForbidden, "the author or a moderator can do that")
		return
	}
	var in struct {
		On bool `json:"on"`
	}
	_ = decode(r, &in)
	if err := s.store.SetWiki(ctx, p.ID, in.On); err != nil {
		s.fail(w, err, "wiki")
		return
	}
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	writeJSON(w, http.StatusOK, map[string]any{"wiki": in.On})
}

// handleHidePost is a moderator hiding (or showing again) a post.
func (s *Server) handleHidePost(w http.ResponseWriter, r *http.Request) {
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
		Hidden bool   `json:"hidden"`
		Reason string `json:"reason"`
	}
	_ = decode(r, &in)
	state := "visible"
	if in.Hidden {
		state = "hidden"
	}
	if err := s.store.SetPostState(ctx, p, state, cleanText(in.Reason, 200)); err != nil {
		s.fail(w, err, "hide")
		return
	}
	if p.Number == 1 {
		t.State = state
		_ = s.store.UpdateTopic(ctx, t)
	}
	if in.Hidden && p.UserID > 0 && p.UserID != a.uid() {
		s.addNotification(ctx, &store.Notification{UserID: p.UserID, ForumID: a.f.ID, Kind: "moderation", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: a.uid(), ActorName: s.actorName(a.u),
			Title: "A moderator hid your post in “" + t.Title + "”", Excerpt: p.HiddenReason})
	}
	s.modLog(ctx, a, map[bool]string{true: "hid a post", false: "showed a post"}[in.Hidden], "post", p.ID, t.Title+" #"+strconv.Itoa(p.Number)+" "+p.HiddenReason)
	s.live(a.f.ID, map[string]any{"type": "edit", "topic_id": t.ID, "post_id": p.ID, "number": p.Number})
	writeJSON(w, http.StatusOK, map[string]any{"state": state})
}

// ---- polls ----

func (s *Server) pollParam(w http.ResponseWriter, r *http.Request) (*access, *store.Poll, bool) {
	ctx := r.Context()
	u := userFrom(ctx)
	p, err := s.store.GetPoll(ctx, idParam(r, "id"), u.ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	a, _, err := s.topicAccess(ctx, u, p.TopicID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return a, p, true
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, p, ok := s.pollParam(w, r)
	if !ok {
		return
	}
	if !a.member() {
		writeErr(w, http.StatusForbidden, "join the forum to vote")
		return
	}
	if p.Closed || (p.ClosesAt != nil && *p.ClosesAt <= store.Now()) {
		writeErr(w, http.StatusBadRequest, "this poll is closed")
		return
	}
	var in struct {
		Choices []int `json:"choices"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var ch []int
	for _, c := range in.Choices {
		if c >= 0 && c < len(p.Options) && !containsInt(ch, c) {
			ch = append(ch, c)
		}
	}
	if !p.Multi && len(ch) > 1 {
		ch = ch[:1]
	}
	if err := s.store.Vote(ctx, p.ID, a.uid(), ch); err != nil {
		s.fail(w, err, "vote")
		return
	}
	p, _ = s.store.GetPoll(ctx, p.ID, a.uid())
	s.live(a.f.ID, map[string]any{"type": "poll", "topic_id": p.TopicID, "post_id": p.PostID})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleClosePoll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, p, ok := s.pollParam(w, r)
	if !ok {
		return
	}
	if !a.mod() && p.UserID != a.uid() {
		writeErr(w, http.StatusForbidden, "the person who made the poll (or a moderator) can close it")
		return
	}
	var in struct {
		Closed bool `json:"closed"`
	}
	_ = decode(r, &in)
	if err := s.store.ClosePoll(ctx, p.ID, in.Closed); err != nil {
		s.fail(w, err, "poll")
		return
	}
	p, _ = s.store.GetPoll(ctx, p.ID, a.uid())
	writeJSON(w, http.StatusOK, p)
}

func containsInt(v []int, x int) bool {
	for _, y := range v {
		if y == x {
			return true
		}
	}
	return false
}
