package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// The other Pocket apps. None of it is needed: when an app is not there,
// the page greys the button and says what to do.

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

func plainErr(a pocketApp, err error) string {
	if isNotConnected(err) {
		return err.Error()
	}
	return a.name + ": " + err.Error()
}

func (s *Server) appFail(w http.ResponseWriter, a pocketApp, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeErr(w, http.StatusBadGateway, plainErr(a, err))
}

// handleApps lists how the person reaches each app, in words they can act on.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

func (s *Server) sib(w http.ResponseWriter, r *http.Request, a pocketApp, scopes ...string) (*siblingConn, bool) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, a, scopes...)
	if err != nil {
		s.appFail(w, a, err)
		return nil, false
	}
	return sib, true
}

// topicFrom reads the topic (and optionally one of its posts) a request
// is about, for the person.
type aboutIn struct {
	TopicID int64  `json:"topic_id"`
	PostID  int64  `json:"post_id"`
	Text    string `json:"text"`
	To      string `json:"to"`
	Deck    string `json:"deck"`
	TeamID  int64  `json:"team_id"`
	Due     string `json:"due"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func (s *Server) about(w http.ResponseWriter, r *http.Request) (*aboutIn, *access, *store.Topic, *store.Post, bool) {
	ctx := r.Context()
	var in aboutIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return nil, nil, nil, nil, false
	}
	tid := in.TopicID
	var p *store.Post
	if in.PostID > 0 {
		var err error
		p, err = s.store.GetPost(ctx, in.PostID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return nil, nil, nil, nil, false
		}
		tid = p.TopicID
	}
	a, t, err := s.topicAccess(ctx, userFrom(ctx), tid)
	if err != nil || (p != nil && p.State != "visible") {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, nil, nil, false
	}
	return &in, a, t, p, true
}

func (s *Server) topicURL(t *store.Topic, number int) string {
	if number > 1 {
		return s.link(fmt.Sprintf("/t/%d/%d", t.ID, number))
	}
	return s.link(fmt.Sprintf("/t/%d", t.ID))
}

// topicMarkdown is a whole topic as Markdown (for a note, a file, a blog draft).
func (s *Server) topicMarkdown(ctx context.Context, a *access, t *store.Topic, withLinks bool) string {
	posts, _ := s.store.ListPosts(ctx, t.ID, 0, false, 0, 1000)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.Title)
	fmt.Fprintf(&b, "*%s %s · %d %s*\n\n", a.f.Emoji, a.f.Name, len(posts), plural(len(posts), "post", "posts"))
	for _, p := range posts {
		if p.State != "visible" {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, p.CreatedAt)
		mark := ""
		if p.ID == t.SolvedPostID {
			mark = " ✅ solution"
		}
		fmt.Fprintf(&b, "---\n\n**%s** · %s%s\n\n%s\n\n", p.AuthorName, at.Local().Format("2 Jan 2006 15:04"), mark, mdPlain(absUploads(p.Body, s.cfg.BaseURL)))
	}
	if withLinks {
		fmt.Fprintf(&b, "---\n\n[Open the topic](%s)\n", s.topicURL(t, 0))
	}
	return b.String()
}

func absUploads(body, base string) string {
	return strings.ReplaceAll(body, "](/api/v1/uploads/", "]("+strings.TrimRight(base, "/")+"/api/v1/uploads/")
}

// ---- Pocket Chat ----

func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appChat, scopeChatRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/conversations", nil)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "conversations") {
		if m["archived"] == true {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "name"), str(m, "title")), "kind": str(m, "kind"), "emoji": str(m, "emoji")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// handleShareChat posts a topic (or one post of it) to a chat as a card with its link.
func (s *Server) handleShareChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, a, t, p, ok := s.about(w, r)
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appChat, scopeChatWrite)
	if !ok {
		return
	}
	n := 0
	line := excerpt(t.Excerpt, 160)
	if p != nil {
		n, line = p.Number, p.AuthorName+": "+excerpt(p.Body, 160)
	}
	sub := a.f.Emoji + " " + a.f.Name
	if t.PostsN > 1 {
		sub += fmt.Sprintf(" · %d %s", t.PostsN-1, plural(t.PostsN-1, "reply", "replies"))
	}
	card := map[string]any{"title": t.Title, "subtitle": sub, "lines": []string{line}, "icon": "💬", "url": s.topicURL(t, n), "app": "forum", "kind": "card"}
	st := s.settingsOf(ctx, a.uid())
	body := map[string]any{"to": firstNonEmpty(strings.TrimSpace(in.To), st.ChatTo, "me"), "text": clip(firstNonEmpty(strings.TrimSpace(in.Text), "Have a look at this in the forum"), 2000), "card": card,
		"client_id": fmt.Sprintf("forum-%d-%d-%d-%d", a.uid(), t.ID, n, time.Now().Unix()/30)}
	res, err := postJSONTo(ctx, sib, "/api/v1/send", body)
	if err != nil {
		s.appFail(w, appChat, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/c/"+str(res, "conversation_id")), "text": "Sent to Pocket Chat"})
}

// ---- Pocket Email ----

// handleShareEmail emails a topic, or an invitation, from the person's own mail account.
func (s *Server) handleShareEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		TopicID  int64  `json:"topic_id"`
		InviteID int64  `json:"invite_id"`
		Text     string `json:"text"`
		To       []struct {
			Name string `json:"name"`
			Addr string `json:"addr"`
		} `json:"to"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	to := []map[string]string{}
	for _, t := range in.To {
		if a := strings.TrimSpace(t.Addr); strings.Contains(a, "@") && len(to) < 50 {
			to = append(to, map[string]string{"name": strings.TrimSpace(t.Name), "addr": a})
		}
	}
	if len(to) == 0 {
		writeErr(w, http.StatusBadRequest, "add at least one email address")
		return
	}
	var subject, text, link, title string
	switch {
	case in.InviteID > 0:
		i, err := s.store.GetInvite(ctx, in.InviteID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		a, err := s.forumAccess(ctx, u, strconv.FormatInt(i.ForumID, 10))
		if err != nil || !a.mod() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if why := inviteUsable(i); why != "" {
			writeErr(w, http.StatusBadRequest, why)
			return
		}
		link, title = s.link("/join/"+i.Token), a.f.Name
		subject = s.actorName(u) + " invites you to " + a.f.Name
		text = firstNonEmpty(strings.TrimSpace(in.Text), "I would like you to join "+a.f.Name+", our forum.")
		if a.f.About != "" {
			text += "\n\n" + a.f.About
		}
	case in.TopicID > 0:
		a, t, err := s.topicAccess(ctx, u, in.TopicID)
		if err != nil || t.State != "visible" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		link, title = s.topicURL(t, 0), t.Title
		if a.f.Visibility == "public" {
			link = s.link(fmt.Sprintf("/p/%s/t/%d", a.f.Slug, t.ID))
		}
		subject = t.Title
		text = firstNonEmpty(strings.TrimSpace(in.Text), "Have a look at this in "+a.f.Name+":") + "\n\n" + excerpt(t.Excerpt, 400)
	default:
		writeErr(w, http.StatusBadRequest, "send a topic or an invitation")
		return
	}
	sib, ok := s.sib(w, r, appEmail, scopeMailSend)
	if !ok {
		return
	}
	textBody := text + "\n\n" + link
	html := `<div style="font-family:-apple-system,Segoe UI,Roboto,sans-serif;line-height:1.5">` + strings.ReplaceAll(htmlEsc(text), "\n", "<br>") +
		`<p><a href="` + htmlEsc(link) + `" style="display:inline-block;padding:10px 16px;background:#4361c2;color:#fff;border-radius:8px;text-decoration:none">` + htmlEsc(map[bool]string{true: "Join " + title, false: "Open “" + title + "”"}[in.InviteID > 0]) + `</a></p></div>`
	if _, err := postJSONTo(ctx, sib, "/api/v1/send", map[string]any{"to": to, "subject": clip(subject, 200), "body_text": textBody, "body_html": html}); err != nil {
		s.appFail(w, appEmail, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": fmt.Sprintf("Sent to %d %s", len(to), plural(len(to), "person", "people"))})
}

// ---- Pocket Contacts ----

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appContacts, scopeContactsRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"40"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/contacts", q)
	if err != nil {
		s.appFail(w, appContacts, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "contacts") {
		emails := []string{}
		if arr, ok := m["emails"].([]any); ok {
			for _, e := range arr {
				switch x := e.(type) {
				case string:
					emails = append(emails, x)
				case map[string]any:
					if v := str(x, "value", "email", "addr"); v != "" {
						emails = append(emails, v)
					}
				}
			}
		}
		if len(emails) == 0 {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "fn"), str(m, "name"), str(m, "given")), "emails": emails})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// ---- Pocket Calendar ----

// handleToCalendar puts an event topic in the person's calendar.
func (s *Server) handleToCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, a, t, _, ok := s.about(w, r)
	if !ok {
		return
	}
	if t.Kind != "event" || t.EventStart == "" {
		writeErr(w, http.StatusBadRequest, "only an event topic has a date to add")
		return
	}
	body := map[string]any{"title": clip(t.Title, 200), "description": excerpt(t.Excerpt, 600) + "\n\nIn the forum: " + s.topicURL(t, 0), "url": s.topicURL(t, 0), "categories": []string{a.f.Name}}
	if t.EventPlace != "" {
		body["location"] = t.EventPlace
	}
	if len(t.EventStart) == 10 {
		end := firstNonEmpty(t.EventEnd, t.EventStart)
		body["all_day"], body["start"], body["end"] = true, t.EventStart, end[:10]
	} else {
		start, _ := time.ParseInLocation("2006-01-02T15:04", t.EventStart, time.Local)
		end := start.Add(2 * time.Hour)
		if e, err := time.ParseInLocation("2006-01-02T15:04", t.EventEnd, time.Local); err == nil && e.After(start) {
			end = e
		}
		body["start"], body["end"] = start.Format("2006-01-02T15:04"), end.Format("2006-01-02T15:04")
	}
	sib, ok := s.sib(w, r, appCalendar, scopeCalendarWrite)
	if !ok {
		return
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		s.appFail(w, appCalendar, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/?event="+url.QueryEscape(firstNonEmpty(str(res, "event_id"), idOf(res)))), "text": "Added to Pocket Calendar"})
}

// ---- Pocket Tasks ----

func (s *Server) handleToTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, a, t, p, ok := s.about(w, r)
	if !ok {
		return
	}
	if in.Due != "" && !validDate(in.Due) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	sib, ok := s.sib(w, r, appTasks, scopeTasksWrite)
	if !ok {
		return
	}
	n := 0
	notes := excerpt(t.Excerpt, 500)
	if p != nil {
		n = p.Number
		notes = p.AuthorName + ": " + excerpt(p.Body, 500)
	}
	title := firstNonEmpty(cleanText(in.Title, 200), "Reply to “"+t.Title+"”")
	task, err := sib.client.CreateTask(ctx, clip(title, 200), notes+"\n\n"+a.f.Name+" — Pocket Forum", in.Due, s.topicURL(t, n))
	if err != nil {
		s.appFail(w, appTasks, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Added to Pocket Tasks", "url": linkTo(sib, "/tasks/"+strconv.FormatInt(task.ID, 10))})
}

// ---- Pocket Projects ----

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appProjects, scopeProjectsRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/teams", nil)
	if err != nil {
		s.appFail(w, appProjects, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "teams") {
		out = append(out, map[string]any{"id": idOf(m), "name": str(m, "name"), "key": str(m, "key", "identifier")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"teams": out})
}

// handleToProjects turns a topic into an issue, with the forum link.
func (s *Server) handleToProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, a, t, p, ok := s.about(w, r)
	if !ok {
		return
	}
	team := in.TeamID
	if team == 0 {
		team = s.settingsOf(ctx, a.uid()).ProjectTeam
	}
	if team == 0 {
		writeErr(w, http.StatusBadRequest, "pick the team the issue goes to")
		return
	}
	sib, ok := s.sib(w, r, appProjects, scopeProjectsWrite)
	if !ok {
		return
	}
	first, _ := s.store.PostByNumber(ctx, t.ID, 1)
	desc := ""
	if first != nil {
		desc = mdPlain(absUploads(first.Body, s.cfg.BaseURL))
	}
	n := 0
	if p != nil && p.Number > 1 {
		n = p.Number
		desc += "\n\n---\n**" + p.AuthorName + "** (post #" + strconv.Itoa(p.Number) + "):\n\n" + mdPlain(absUploads(p.Body, s.cfg.BaseURL))
	}
	desc += fmt.Sprintf("\n\n---\nFrom %s %s in Pocket Forum: %s", a.f.Emoji, a.f.Name, s.topicURL(t, n))
	res, err := postJSONTo(ctx, sib, "/api/v1/issues", map[string]any{"team_id": team, "title": clip(firstNonEmpty(cleanText(in.Title, 200), t.Title), 200), "description": clip(desc, 50000)})
	if err != nil {
		s.appFail(w, appProjects, err)
		return
	}
	ident := firstNonEmpty(str(res, "identifier"), idOf(res))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Issue " + ident + " made in Pocket Projects", "url": linkTo(sib, "/issue/"+url.PathEscape(ident)), "identifier": ident})
}

// ---- Pocket Notes ----

func (s *Server) handleNotesSearch(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appNotes, scopeNotesRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/search", url.Values{"q": {r.URL.Query().Get("q")}, "limit": {"30"}})
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "hits", "notes") {
		n := m
		if inner, ok := m["note"].(map[string]any); ok {
			n = inner
		}
		out = append(out, map[string]any{"id": idOf(n), "title": firstNonEmpty(str(n, "title"), "Untitled"), "folder": str(n, "category"), "snippet": clip(firstNonEmpty(str(m, "snippet"), str(n, "excerpt")), 200)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": out})
}

func (s *Server) handleNoteGet(w http.ResponseWriter, r *http.Request) {
	id := chiParam(r, "id")
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appNotes, scopeNotesRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/notes/"+id, nil)
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	m, _ := v.(map[string]any)
	if inner, ok := m["note"].(map[string]any); ok {
		m = inner
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": str(m, "title"), "content": clip(str(m, "content"), maxBody-500), "url": linkTo(sib, "/notes/"+id)})
}

// handleToNotes saves a topic (or a summary of it) as a note.
func (s *Server) handleToNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, a, t, _, ok := s.about(w, r)
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appNotes, scopeNotesWrite)
	if !ok {
		return
	}
	content := s.topicMarkdown(ctx, a, t, true)
	title := t.Title
	if sum := strings.TrimSpace(in.Summary); sum != "" {
		content = "# " + t.Title + "\n\n" + clip(sum, 20000) + "\n\n[Open the topic](" + s.topicURL(t, 0) + ")\n"
		title += " — summary"
	}
	folder := firstNonEmpty(s.settingsOf(ctx, a.uid()).NotesFolder, "Forum")
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(title, 200), "content": content, "category": folder})
	if err != nil {
		s.appFail(w, appNotes, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": linkTo(sib, "/notes/"+idOf(res)), "text": "Saved in Pocket Notes"})
}

// ---- Pocket Files ----

func (s *Server) handleToFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, a, t, _, ok := s.about(w, r)
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appFiles, scopeFilesWrite)
	if !ok {
		return
	}
	folder := strings.Trim(firstNonEmpty(s.settingsOf(ctx, a.uid()).FilesFolder, "Forum"), "/")
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, clip(t.Title, 90)) + ".md"
	var res map[string]any
	q := url.Values{"space": {"home"}, "path": {"/"}, "name": {folder + "/" + name}, "conflict": {"rename"}, "mtime": {strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	if err := sib.client.PutRaw(ctx, "/api/v1/fs/upload", q, []byte(s.topicMarkdown(ctx, a, t, true)), "text/markdown", &res); err != nil {
		s.appFail(w, appFiles, err)
		return
	}
	path := firstNonEmpty(str(res, "path"), "/"+folder+"/"+name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Saved in Pocket Files", "url": linkTo(sib, "/f/home"+path)})
}

// ---- Pocket Photos ----

func (s *Server) handlePhotosList(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	body := map[string]any{"filter": map[string]any{"trashedAt": map[string]any{"eq": nil}, "type": map[string]any{"eq": "IMAGE"}, "visibility": map[string]any{"eq": "timeline"}},
		"size": 60, "withExif": false, "orderBy": map[string]any{"field": "fileCreatedAt", "direction": "desc"}}
	path := "/api/v1/immich/search/metadata"
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		body["query"] = q
		path = "/api/v1/immich/search/smart"
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		body["cursor"] = c
	}
	res, err := postJSONTo(r.Context(), sib, path, body)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	assets := obj(res, "assets")
	out := []map[string]any{}
	for _, m := range rows(assets["items"]) {
		id := str(m, "id")
		if !reUUID.MatchString(id) {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": str(m, "originalFileName"), "date": day10(m["fileCreatedAt"]), "thumb_url": "/api/v1/apps/photos/" + id + "/image"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"photos": out, "cursor": assets["nextCursor"]})
}

func (s *Server) handlePhotoImage(w http.ResponseWriter, r *http.Request) {
	id := chiParam(r, "uuid")
	if !reUUID.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	b, ctype, err := sib.client.Bytes(r.Context(), "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"thumbnail"}}, 16<<20)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	w.Header().Set("Content-Type", firstNonEmpty(ctype, http.DetectContentType(b)))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// handlePhotoCopy puts a copy of a Pocket Photos picture in a forum, for a post.
func (s *Server) handlePhotoCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	id := chiParam(r, "uuid")
	if !reUUID.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	a, err := s.forumAccess(ctx, u, r.URL.Query().Get("forum"))
	if err != nil || !a.member() {
		writeErr(w, http.StatusForbidden, "join the forum to add pictures")
		return
	}
	sib, ok := s.sib(w, r, appPhotos, scopePhotosRead)
	if !ok {
		return
	}
	b, ctype, err := sib.client.Bytes(ctx, "/api/v1/immich/assets/"+id+"/thumbnail", url.Values{"size": {"preview"}}, maxUpload)
	if err != nil {
		s.appFail(w, appPhotos, err)
		return
	}
	mime := firstNonEmpty(strings.Split(ctype, ";")[0], http.DetectContentType(b))
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	up := &store.Upload{UserID: u.ID, ForumID: a.f.ID, Name: "photo.jpg", Mime: mime, Data: b}
	if err := s.store.SaveUpload(ctx, up); err != nil {
		s.fail(w, err, "photo")
		return
	}
	url := fmt.Sprintf("/api/v1/uploads/%d", up.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"upload": up, "url": url, "markdown": "![photo](" + url + ")"})
}

// ---- Pocket Learn ----

func (s *Server) handleLearnDecks(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appLearn, scopeLearnRead)
	if !ok {
		return
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/decks", nil)
	if err != nil {
		s.appFail(w, appLearn, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "decks") {
		if m["archived"] == true {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": str(m, "name")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": out})
}

// handleToLearn turns a solved question into a flashcard: the question on
// the front, the accepted answer on the back.
func (s *Server) handleToLearn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, a, t, p, ok := s.about(w, r)
	if !ok {
		return
	}
	ans := p
	if ans == nil && t.SolvedPostID > 0 {
		ans, _ = s.store.GetPost(ctx, t.SolvedPostID)
	}
	if ans == nil || ans.Number == 1 {
		writeErr(w, http.StatusBadRequest, "pick the reply that answers it (or mark the solution first)")
		return
	}
	sib, ok := s.sib(w, r, appLearn, scopeLearnWrite)
	if !ok {
		return
	}
	deck := firstNonEmpty(strings.TrimSpace(in.Deck), s.settingsOf(ctx, a.uid()).LearnDeck, "Forum answers")
	ref := fmt.Sprintf("forum:%d:%d", t.ID, ans.ID)
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"deck": deck, "kind": "basic", "front": t.Title, "back": clip(mdPlain(ans.Body), 4000),
		"external_id": ref, "source_ref": ref, "source_url": s.topicURL(t, ans.Number)})
	if err != nil {
		s.appFail(w, appLearn, err)
		return
	}
	text := "Card added to “" + deck + "” in Pocket Learn"
	if res["duplicate"] == true {
		text = "That card is already in “" + deck + "”"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": text, "url": linkTo(sib, "/")})
}

// ---- Pocket Blog ----

// handleToBlog starts a blog post draft from a topic, crediting its people.
func (s *Server) handleToBlog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, a, t, _, ok := s.about(w, r)
	if !ok {
		return
	}
	sib, ok := s.sib(w, r, appBlog, scopeBlogWrite)
	if !ok {
		return
	}
	first, _ := s.store.PostByNumber(ctx, t.ID, 1)
	body := ""
	if first != nil {
		body = absUploads(first.Body, s.cfg.BaseURL)
	}
	names := []string{}
	for _, p := range t.Posters {
		names = append(names, p.Name)
	}
	link := s.topicURL(t, 0)
	if a.f.Visibility == "public" {
		link = s.link(fmt.Sprintf("/p/%s/t/%d", a.f.Slug, t.ID))
	}
	body += "\n\n---\n*From the discussion in " + a.f.Name + " with " + strings.Join(names, ", ") + ". [Join in](" + link + ").*\n"
	res, err := postJSONTo(ctx, sib, "/api/v1/posts", map[string]any{"title": clip(t.Title, 200), "body": mdPlain(body), "excerpt": excerpt(t.Excerpt, 200), "tags": t.Tags,
		"source_ref": fmt.Sprintf("forum:%d", t.ID)})
	if err != nil {
		s.appFail(w, appBlog, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "A draft post is waiting in Pocket Blog", "url": linkTo(sib, "/posts/"+idOf(res))})
}

// ---- Pocket Bookshelf and Places: things to put in a post ----

func (s *Server) handleBooks(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appBookshelf, scopeBooksRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"60"}, "sort": {"recent"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/books", q)
	if err != nil {
		s.appFail(w, appBookshelf, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "books") {
		author := ""
		if a, ok := m["authors"].([]any); ok && len(a) > 0 {
			author, _ = a[0].(string)
		} else {
			author = str(m, "authors", "author")
		}
		out = append(out, map[string]any{"id": idOf(m), "title": str(m, "title"), "author": author, "status": str(m, "status"), "url": linkTo(sib, "/books/"+idOf(m))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"books": out})
}

func (s *Server) handlePlaces(w http.ResponseWriter, r *http.Request) {
	sib, ok := s.sib(w, r, appPlaces, scopePlacesRead)
	if !ok {
		return
	}
	q := url.Values{"limit": {"80"}}
	if t := strings.TrimSpace(r.URL.Query().Get("q")); t != "" {
		q.Set("q", t)
	}
	v, err := getJSONFrom(r.Context(), sib, "/api/v1/places", q)
	if err != nil {
		s.appFail(w, appPlaces, err)
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "places") {
		out = append(out, map[string]any{"id": idOf(m), "name": str(m, "name"), "address": joinNonEmpty(", ", str(m, "address"), str(m, "city")), "kind": str(m, "kind"), "url": linkTo(sib, "/places/"+idOf(m))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": out})
}

// ---- Pocket Translate (or the AI) ----

// handleTranslate puts a post into the person's language: Pocket Translate
// when it is connected, else the AI.
func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		PostID int64  `json:"post_id"`
		To     string `json:"to"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.To) == "" {
		writeErr(w, http.StatusBadRequest, "send the post and the language")
		return
	}
	p, err := s.store.GetPost(ctx, in.PostID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if _, _, err := s.topicAccess(ctx, userFrom(ctx), p.TopicID); err != nil || p.State != "visible" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	text := p.Body
	if sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appTranslate, scopeTranslate); err == nil {
		res, err := postJSONTo(ctx, sib, "/api/v1/translate", map[string]any{"texts": []string{text}, "to": in.To, "from": "auto"})
		if err == nil {
			if arr, ok := res["texts"].([]any); ok && len(arr) == 1 {
				if v, ok := arr[0].(string); ok {
					writeJSON(w, http.StatusOK, map[string]any{"text": v, "by": "Pocket Translate"})
					return
				}
			}
		}
	}
	st := s.settingsOf(ctx, actorID(ctx))
	cl, _, why := s.aiFor(ctx, actorID(ctx), st)
	if cl == nil {
		writeErr(w, http.StatusBadRequest, firstNonEmpty("connect Pocket Translate or an AI to translate: "+why, "connect Pocket Translate or an AI to translate"))
		return
	}
	out, err := cl.Complete(ctx, "Translate the forum post the user sends into the language "+in.To+". Keep its Markdown, @mentions, links and [quote=…] blocks exactly as they are. Answer with the translation only.", text, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(out), "by": "AI"})
}
