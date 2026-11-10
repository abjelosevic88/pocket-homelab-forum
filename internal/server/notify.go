package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// ---- telling people ----
//
// A new post tells, once each and in this order of importance: the people
// @mentioned in it, the author of the post it answers or quotes, then
// everyone watching the topic (or, for a new topic, watching its category
// or the whole forum). Nobody hears about their own post, and nobody is
// told about a forum they cannot read.

var kindRank = map[string]int{"mention": 5, "quote": 4, "reply": 3, "watching": 2, "new_topic": 1}

// notifyPost fans a new visible post out to the people it concerns.
func (s *Server) notifyPost(ctx context.Context, a *access, t *store.Topic, p *store.Post, isTopic bool) {
	ctx = context.WithoutCancel(ctx)
	who := map[int64]string{}
	set := func(uid int64, kind string) {
		if uid == 0 || uid == p.UserID {
			return
		}
		if kindRank[kind] > kindRank[who[uid]] {
			who[uid] = kind
		}
	}
	for _, h := range mentions(p.Body) {
		if pr, err := s.store.ProfileByHandle(ctx, h); err == nil {
			set(pr.UserID, "mention")
		}
	}
	for h, n := range quoted(p.Body) {
		if pr, err := s.store.ProfileByHandle(ctx, h); err == nil && n > 0 {
			set(pr.UserID, "quote")
		}
	}
	if p.ReplyTo > 0 {
		if q, err := s.store.PostByNumber(ctx, t.ID, p.ReplyTo); err == nil {
			set(q.UserID, "reply")
		}
	} else if !isTopic && t.UserID > 0 {
		// a reply to the topic itself: its author hears about it like a reply
		set(t.UserID, "reply")
	}
	for _, uid := range s.store.Watchers(ctx, t.ID) {
		set(uid, "watching")
	}
	if isTopic {
		for _, uid := range s.store.UsersWatchingCategory(ctx, t.ForumID, t.CategoryID) {
			set(uid, "new_topic")
		}
	}
	for uid, kind := range who {
		if !s.mayRead(ctx, uid, a.f) {
			continue
		}
		if l := s.store.TopicLevel(ctx, t.ID, uid); l == "muted" && kind != "mention" {
			continue
		}
		s.addNotification(ctx, &store.Notification{UserID: uid, ForumID: t.ForumID, Kind: kind, TopicID: t.ID, PostID: p.ID, Number: p.Number,
			ActorID: p.UserID, ActorName: p.AuthorName, Title: t.Title, Excerpt: excerpt(p.Body, 200)})
		if kind == "mention" {
			s.publish(ctx, uid, "forum.mentioned", t.Title, map[string]any{"forum": a.f.Slug, "topic_id": t.ID, "post_id": p.ID, "by": p.AuthorName, "url": s.link(fmt.Sprintf("/t/%d/%d", t.ID, p.Number))})
		}
	}
}

// mayRead is whether a person (by id) can read a forum.
func (s *Server) mayRead(ctx context.Context, uid int64, f *store.Forum) bool {
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return false
	}
	a, err := s.accessTo(ctx, u, f)
	return err == nil && a.canRead()
}

// addNotification stores it and lights up the person's bell at once.
func (s *Server) addNotification(ctx context.Context, n *store.Notification) {
	if _, err := s.store.AddNotification(ctx, n); err != nil {
		s.log.Warn("notification", "err", err)
		return
	}
	s.hub.send([]int64{n.UserID}, map[string]any{"type": "notification", "unread": s.store.UnreadNotifications(ctx, n.UserID), "kind": n.Kind, "topic_id": n.TopicID})
}

// tellMods tells a forum's moderators about something waiting for them.
func (s *Server) tellMods(ctx context.Context, f *store.Forum, n store.Notification, except int64) {
	for _, uid := range s.store.MemberIDs(ctx, f.ID, true) {
		if uid == except {
			continue
		}
		x := n
		x.UserID, x.ForumID = uid, f.ID
		s.addNotification(ctx, &x)
		s.hub.send([]int64{uid}, map[string]any{"type": "review"})
	}
}

// pushWanted says whether a person wants a phone push for this kind.
func pushWanted(st store.Settings, kind string) bool {
	switch kind {
	case "mention", "quote":
		return st.NotifyMentions
	case "reply":
		return st.NotifyReplies
	case "watching":
		return st.NotifyWatching
	case "new_topic":
		return st.NotifyNewTopics
	case "flag", "pending", "join_request":
		return st.NotifyReview
	case "reaction", "solved":
		return st.NotifyLikes
	case "reminder", "moderation", "joined", "welcome":
		return true
	}
	return false
}

// pushPending sends phone pushes for notifications at least 20 seconds old
// that nobody has seen; several for one person go out as one message.
// Someone with the forum open in a browser already saw them.
func (s *Server) pushPending(ctx context.Context) {
	list, err := s.store.UnpushedNotifications(ctx, time.Now().Add(-20*time.Second).UTC().Format(time.RFC3339Nano))
	if err != nil || len(list) == 0 {
		return
	}
	by := map[int64][]store.Notification{}
	for _, n := range list {
		s.store.MarkPushed(ctx, n.ID)
		by[n.UserID] = append(by[n.UserID], n)
	}
	for uid, ns := range by {
		if s.hub.online(uid) {
			continue
		}
		st := s.settingsOf(ctx, uid)
		if st.NotifyURL == "" {
			continue
		}
		var want []store.Notification
		for _, n := range ns {
			if pushWanted(st, n.Kind) {
				want = append(want, n)
			}
		}
		if len(want) == 0 {
			continue
		}
		if len(want) == 1 {
			n := want[0]
			s.push(ctx, uid, st, notifTitle(n), n.Excerpt, s.link(notifPath(n)))
			continue
		}
		lines := []string{}
		for i, n := range want {
			if i == 6 {
				lines = append(lines, fmt.Sprintf("…and %d more", len(want)-6))
				break
			}
			lines = append(lines, "• "+notifTitle(n))
		}
		s.push(ctx, uid, st, fmt.Sprintf("%d new in your forums", len(want)), strings.Join(lines, "\n"), s.link("/inbox"))
	}
}

func notifPath(n store.Notification) string {
	switch {
	case n.Kind == "join_request" && n.ForumSlug != "":
		return "/review"
	case n.Kind == "flag" || n.Kind == "pending":
		return "/review"
	case n.TopicID > 0 && n.Number > 0:
		return fmt.Sprintf("/t/%d/%d", n.TopicID, n.Number)
	case n.TopicID > 0:
		return fmt.Sprintf("/t/%d", n.TopicID)
	case n.ForumSlug != "":
		return "/f/" + n.ForumSlug
	}
	return "/inbox"
}

// notifTitle is a notification in words.
func notifTitle(n store.Notification) string {
	who := firstNonEmpty(n.ActorName, "Someone")
	switch n.Kind {
	case "mention":
		return who + " mentioned you in “" + n.Title + "”"
	case "quote":
		return who + " quoted you in “" + n.Title + "”"
	case "reply":
		return who + " replied in “" + n.Title + "”"
	case "watching":
		return who + " posted in “" + n.Title + "”"
	case "new_topic":
		return who + " started “" + n.Title + "”"
	case "reaction":
		return who + " reacted to your post in “" + n.Title + "”"
	case "solved":
		return who + " marked your answer as the solution in “" + n.Title + "”"
	case "join_request":
		return who + " asks to join " + n.ForumName
	case "joined":
		return "You are in " + n.ForumName
	case "welcome":
		return "Welcome to " + n.ForumName
	case "flag":
		return who + " flagged a post in “" + n.Title + "”"
	case "pending":
		return "A post waits for approval in “" + n.Title + "”"
	case "moderation":
		return n.Title
	case "reminder":
		return "Reminder: “" + n.Title + "”"
	}
	return n.Title
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"speech_balloon"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Forum", Body: notifyTestBody, Priority: "info", Tags: []string{"speech_balloon"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. You hear when someone mentions you, answers you or posts in a topic you watch — unless the forum is open in front of you."

// ---- timers ----

// timers closes topics whose time ran out, unpins the ones pinned until a
// date, closes polls and sends bookmark reminders.
func (s *Server) timers(ctx context.Context) {
	now := store.Now()
	rows, err := s.store.DB.QueryContext(ctx, `SELECT id FROM topics WHERE (close_at IS NOT NULL AND close_at <= ? AND closed = 0) OR (pinned_until IS NOT NULL AND pinned_until <= ? AND pinned > 0)`, now, now)
	if err == nil {
		var ids []int64
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			t, err := s.store.GetTopic(ctx, id)
			if err != nil {
				continue
			}
			if t.CloseAt != nil && *t.CloseAt <= now && !t.Closed {
				t.Closed, t.CloseAt = true, nil
				s.store.Log(ctx, store.LogEntry{ForumID: t.ForumID, ActorName: "Timer", Action: "closed", TargetKind: "topic", TargetID: t.ID, Detail: t.Title})
			}
			if t.PinnedUntil != nil && *t.PinnedUntil <= now {
				t.Pinned, t.PinnedUntil = 0, nil
			}
			_ = s.store.UpdateTopic(ctx, t)
			s.live(t.ForumID, map[string]any{"type": "topic", "topic_id": t.ID})
		}
	}
	s.store.DuePolls(ctx, now)
	due, users, err := s.store.DueBookmarks(ctx, now)
	if err == nil {
		for i, b := range due {
			s.store.BookmarkReminded(ctx, b.ID)
			s.addNotification(ctx, &store.Notification{UserID: users[i], Kind: "reminder", TopicID: b.TopicID, PostID: b.PostID, Number: b.Number,
				Title: b.TopicTitle, Excerpt: firstNonEmpty(b.Note, excerpt(b.Excerpt, 200))})
		}
	}
}

// ---- summary emails ----

// digests sends, at each person's hour, a summary of what they missed — once
// a day or once a week (their day), and only when they were not here since
// the last one. It goes from their own mail account through Pocket Email.
func (s *Server) digests(ctx context.Context) {
	now := time.Now()
	if now.Minute() > 10 {
		return
	}
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || st.Digest == "off" || now.Hour() != st.NotifyHour {
			continue
		}
		days := 1
		if st.Digest == "weekly" {
			wd := int(now.Weekday())
			if wd == 0 {
				wd = 7
			}
			if wd != st.DigestDay {
				continue
			}
			days = 7
		}
		key := fmt.Sprintf("digest:%d:%s", uid, now.Format("2006-01-02"))
		if _, _, done := s.store.CacheGet(ctx, key); done {
			continue
		}
		_ = s.store.CachePut(ctx, key, "1")
		since := now.AddDate(0, 0, -days)
		if err := s.sendDigest(ctx, uid, since, false); err != nil && !isNotConnected(err) {
			s.log.Warn("summary email", "user", uid, "err", err)
		}
	}
}

// digestFor builds the summary of a person's forums since a time; "" when
// there is nothing to tell (or they were here anyway, unless forced).
func (s *Server) digestFor(ctx context.Context, u *store.User, since time.Time, force bool) (subject, text, htmlBody string) {
	forums, _ := s.store.MyForums(ctx, u.ID)
	sinceS := since.UTC().Format(time.RFC3339Nano)
	var parts []string
	var hparts []string
	total := 0
	for _, f := range forums {
		m, err := s.store.GetMember(ctx, f.ID, u.ID)
		if err != nil || !m.Active() || !m.Digest || f.Archived {
			continue
		}
		if !force && m.LastSeenAt != nil && *m.LastSeenAt >= sinceS {
			continue
		}
		top, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: []int64{f.ID}, Filter: "top", Since: sinceS, Limit: 6})
		open, _ := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: []int64{f.ID}, Filter: "unsolved", Limit: 3})
		if len(top) == 0 {
			continue
		}
		total += len(top)
		lines := []string{f.Emoji + " " + f.Name}
		hl := []string{fmt.Sprintf(`<h3 style="margin:18px 0 6px">%s %s</h3><ul style="padding-left:18px">`, f.Emoji, htmlEsc(f.Name))}
		for _, t := range top {
			url := s.link(fmt.Sprintf("/t/%d", t.ID))
			lines = append(lines, fmt.Sprintf("  • %s (%d %s) %s", t.Title, t.PostsN-1, plural(t.PostsN-1, "reply", "replies"), url))
			hl = append(hl, fmt.Sprintf(`<li><a href="%s">%s</a> <span style="color:#777">· %d %s · %s</span><br><span style="color:#555">%s</span></li>`, url, htmlEsc(t.Title), t.PostsN-1, plural(t.PostsN-1, "reply", "replies"), htmlEsc(t.LastName), htmlEsc(clip(plainText(t.Excerpt), 160))))
		}
		hl = append(hl, "</ul>")
		if len(open) > 0 {
			lines = append(lines, "  Waiting for an answer:")
			hl = append(hl, `<p style="margin:4px 0;color:#555">Waiting for an answer:</p><ul style="padding-left:18px">`)
			for _, t := range open {
				url := s.link(fmt.Sprintf("/t/%d", t.ID))
				lines = append(lines, "  • "+t.Title+" "+url)
				hl = append(hl, fmt.Sprintf(`<li><a href="%s">%s</a></li>`, url, htmlEsc(t.Title)))
			}
			hl = append(hl, "</ul>")
		}
		parts = append(parts, strings.Join(lines, "\n"))
		hparts = append(hparts, strings.Join(hl, ""))
	}
	if total == 0 {
		return "", "", ""
	}
	subject = fmt.Sprintf("What you missed in your forums: %d %s", total, plural(total, "topic", "topics"))
	text = "Here is what happened since " + since.Format("Mon 2 Jan") + ".\n\n" + strings.Join(parts, "\n\n") + "\n\nChange how often this comes in Pocket Forum → Settings → Notifications: " + s.link("/settings/notifications")
	htmlBody = `<div style="font-family:-apple-system,Segoe UI,Roboto,sans-serif;max-width:620px;line-height:1.45"><p>Here is what happened since ` + since.Format("Mon 2 Jan") + `.</p>` +
		strings.Join(hparts, "") + `<p style="color:#888;font-size:13px;margin-top:22px">Change how often this comes in <a href="` + s.link("/settings/notifications") + `">Pocket Forum → Settings → Notifications</a>.</p></div>`
	return subject, text, htmlBody
}

func (s *Server) sendDigest(ctx context.Context, uid int64, since time.Time, force bool) error {
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	subject, text, h := s.digestFor(ctx, u, since, force)
	if subject == "" {
		return &userErr{"nothing new to put in it"}
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appEmail, scopeMailSend)
	if err != nil {
		return err
	}
	_, err = postJSONTo(ctx, sib, "/api/v1/send", map[string]any{"to": []map[string]string{{"name": u.Name, "addr": u.Email}}, "subject": subject, "body_text": text, "body_html": h})
	return err
}

// handleDigestTest sends the summary email now (whatever is new this week).
func (s *Server) handleDigestTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.sendDigest(ctx, actorID(ctx), time.Now().AddDate(0, 0, -7), true); err != nil {
		s.userFail(w, err, "summary email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Sent to " + userFrom(ctx).Email})
}

func htmlEsc(v string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(v)
}

// sortedKeys is a map's keys in order (stable output for tests).
func sortedKeys(m map[int64]string) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
