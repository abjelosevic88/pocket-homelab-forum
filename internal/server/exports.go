package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// ---- export ----
//
// A person's export holds the forums they own, whole (categories, topics,
// posts, polls — authors by name, never their email), plus their profile,
// bookmarks and drafts. Bringing it back merges: topics already there
// (by their key) are left alone, so importing twice changes nothing.

type exportPost struct {
	Key       string         `json:"key"`
	Author    string         `json:"author"`
	Mine      bool           `json:"mine,omitempty"`
	Number    int            `json:"number"`
	ReplyTo   int            `json:"reply_to,omitempty"`
	Body      string         `json:"body"`
	State     string         `json:"state"`
	Wiki      bool           `json:"wiki,omitempty"`
	CreatedAt string         `json:"created_at"`
	Reactions map[string]int `json:"reactions,omitempty"`
	Poll      *store.Poll    `json:"poll,omitempty"`
	Solution  bool           `json:"solution,omitempty"`
}

type exportTopic struct {
	Key        string       `json:"key"`
	Category   string       `json:"category"`
	Title      string       `json:"title"`
	Kind       string       `json:"kind"`
	Closed     bool         `json:"closed,omitempty"`
	Pinned     int          `json:"pinned,omitempty"`
	Tags       []string     `json:"tags"`
	EventStart string       `json:"event_start,omitempty"`
	EventEnd   string       `json:"event_end,omitempty"`
	EventPlace string       `json:"event_place,omitempty"`
	Views      int          `json:"views"`
	CreatedAt  string       `json:"created_at"`
	Posts      []exportPost `json:"posts"`
}

type exportForum struct {
	Forum      store.Forum      `json:"forum"`
	Categories []store.Category `json:"categories"`
	Topics     []exportTopic    `json:"topics"`
}

type exportDoc struct {
	App        string           `json:"app"`
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Profile    *store.Profile   `json:"profile,omitempty"`
	Forums     []exportForum    `json:"forums"`
	Bookmarks  []store.Bookmark `json:"bookmarks"`
	Drafts     []store.Draft    `json:"drafts"`
}

func (s *Server) export(ctx context.Context, u *store.User) (*exportDoc, error) {
	doc := &exportDoc{App: "pocket-forum", Version: Version, ExportedAt: store.Now(), Forums: []exportForum{}}
	doc.Profile, _ = s.store.GetProfile(ctx, u.ID)
	mine, err := s.store.MyForums(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range mine {
		if f.UserID != u.ID {
			continue
		}
		ef := exportForum{Forum: f, Topics: []exportTopic{}}
		ef.Categories, _ = s.store.ListCategories(ctx, f.ID, u.ID)
		cats := map[int64]string{}
		for _, c := range ef.Categories {
			cats[c.ID] = c.Slug
		}
		rows, err := s.store.DB.QueryContext(ctx, `SELECT id FROM topics WHERE forum_id = ? AND state IN ('visible','pending','hidden') ORDER BY id`, f.ID)
		if err != nil {
			return nil, err
		}
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
			key := firstNonEmpty(t.SourceRef, fmt.Sprintf("%s/%d", f.Slug, t.ID))
			if t.Source != "import" && t.Source != "discourse" {
				key = fmt.Sprintf("%s/%d", f.Slug, t.ID)
			}
			et := exportTopic{Key: key, Category: cats[t.CategoryID], Title: t.Title, Kind: t.Kind, Closed: t.Closed, Pinned: t.Pinned, Tags: t.Tags,
				EventStart: t.EventStart, EventEnd: t.EventEnd, EventPlace: t.EventPlace, Views: t.Views, CreatedAt: t.CreatedAt, Posts: []exportPost{}}
			posts, _ := s.store.ListPosts(ctx, t.ID, 0, true, 0, 10000)
			for _, p := range posts {
				if p.State == "deleted" {
					continue
				}
				ep := exportPost{Key: fmt.Sprintf("%d", p.ID), Author: p.AuthorName, Mine: p.UserID == u.ID, Number: p.Number, ReplyTo: p.ReplyTo, Body: p.Body, State: p.State, Wiki: p.Wiki,
					CreatedAt: p.CreatedAt, Reactions: p.Reactions, Solution: p.ID == t.SolvedPostID}
				if poll, err := s.store.PollOfPost(ctx, p.ID, 0); err == nil {
					ep.Poll = poll
				}
				et.Posts = append(et.Posts, ep)
			}
			ef.Topics = append(ef.Topics, et)
		}
		doc.Forums = append(doc.Forums, ef)
	}
	doc.Bookmarks, _ = s.store.ListBookmarks(ctx, u.ID)
	doc.Drafts, _ = s.store.ListDrafts(ctx, u.ID)
	return doc, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := s.export(r.Context(), userFrom(r.Context()))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-forum-%s.json"`, time.Now().Format("20060102")))
	writeJSON(w, http.StatusOK, doc)
}

// handleExportCSV is every topic of the forums the person is in, one per line.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	_, member, _ := s.readableForums(ctx, u)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-forum-topics-%s.csv"`, time.Now().Format("20060102")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"forum", "category", "title", "kind", "started_by", "started", "replies", "views", "reactions", "solved", "closed", "tags", "last_post", "link"})
	for _, fid := range member {
		f, err := s.store.GetForum(ctx, fid)
		if err != nil {
			continue
		}
		cats := map[int64]string{}
		if list, err := s.store.ListCategories(ctx, fid, u.ID); err == nil {
			for _, c := range list {
				cats[c.ID] = c.Name
			}
		}
		for page := 0; page < 200; page++ {
			list, err := s.store.ListTopics(ctx, store.TopicQuery{Viewer: u.ID, ForumIDs: []int64{fid}, Limit: 200, Offset: page * 200})
			if err != nil || len(list) == 0 {
				break
			}
			for _, t := range list {
				last := ""
				if t.LastPostAt != nil {
					last = (*t.LastPostAt)[:10]
				}
				_ = cw.Write([]string{f.Name, cats[t.CategoryID], t.Title, t.Kind, t.AuthorName, t.CreatedAt[:10], strconv.Itoa(t.PostsN - 1), strconv.Itoa(t.Views), strconv.Itoa(t.LikesN),
					strconv.FormatBool(t.SolvedPostID > 0), strconv.FormatBool(t.Closed), strings.Join(t.Tags, ", "), last, s.topicURL(&t, 0)})
			}
			if len(list) < 200 {
				break
			}
		}
	}
	cw.Flush()
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	var b []byte
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, _, ferr := r.FormFile("file")
		if ferr != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		b, err = io.ReadAll(f)
		f.Close()
	} else {
		b, err = io.ReadAll(r.Body)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the file")
		return
	}
	var doc exportDoc
	if err := json.Unmarshal(b, &doc); err != nil || doc.App != "pocket-forum" {
		writeErr(w, http.StatusBadRequest, "this is not a Pocket Forum export")
		return
	}
	res, err := s.importDoc(ctx, u, &doc)
	if err != nil {
		s.userFail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) importDoc(ctx context.Context, u *store.User, doc *exportDoc) (map[string]any, error) {
	forums, topics, posts := 0, 0, 0
	for _, ef := range doc.Forums {
		origin := "import:" + ef.Forum.Slug
		f, err := s.store.GetForumBySlug(ctx, ef.Forum.Slug)
		if err != nil || f.UserID != u.ID {
			f, err = s.store.ForumBySource(ctx, u.ID, origin)
		}
		if err != nil {
			nf := ef.Forum
			nf.ID, nf.UserID, nf.Source = 0, u.ID, origin
			nf.Slug = s.store.FreeForumSlug(ctx, firstNonEmpty(nf.Slug, store.Slugify(nf.Name, "-")), 0)
			cats := []store.Category{}
			for _, c := range ef.Categories {
				if c.ParentID == 0 {
					cats = append(cats, c)
				}
			}
			if err := s.store.CreateForum(ctx, &nf, cats); err != nil {
				return nil, err
			}
			f = &nf
			forums++
		}
		bySlug := map[string]int64{}
		have, _ := s.store.ListCategories(ctx, f.ID, u.ID)
		for _, c := range have {
			bySlug[c.Slug] = c.ID
		}
		for _, c := range ef.Categories {
			if bySlug[c.Slug] == 0 {
				nc := c
				nc.ID, nc.ForumID, nc.ParentID = 0, f.ID, 0
				if err := s.store.SaveCategory(ctx, u.ID, &nc); err == nil {
					bySlug[c.Slug] = nc.ID
				}
			}
		}
		for _, et := range ef.Topics {
			if _, err := s.store.TopicBySource(ctx, f.ID, "import", et.Key); err == nil {
				continue
			}
			if s.sameTopic(ctx, f, et) {
				continue
			}
			n, err := s.importTopic(ctx, f, u, bySlug[et.Category], et, "import")
			if err != nil {
				return nil, err
			}
			topics++
			posts += n
		}
	}
	return map[string]any{"ok": true, "forums": forums, "topics": topics, "posts": posts,
		"text": fmt.Sprintf("Brought back %d %s with %d %s", topics, plural(topics, "topic", "topics"), posts, plural(posts, "post", "posts"))}, nil
}

// sameTopic is true when the export's topic is this very topic: a person
// bringing their own export back into the box it came from (keys of topics
// made here are "<forum slug>/<topic id>").
func (s *Server) sameTopic(ctx context.Context, f *store.Forum, et exportTopic) bool {
	slug, num, ok := strings.Cut(et.Key, "/")
	if !ok || slug != f.Slug {
		return false
	}
	tid, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return false
	}
	t, err := s.store.GetTopic(ctx, tid)
	return err == nil && t.ForumID == f.ID && t.Title == clip(firstNonEmpty(et.Title, "Untitled"), maxTitle)
}

// importTopic adds a topic with its posts in order; posts marked mine are the
// importer's, everyone else's keep their name only.
func (s *Server) importTopic(ctx context.Context, f *store.Forum, u *store.User, cat int64, et exportTopic, source string) (int, error) {
	if len(et.Posts) == 0 {
		return 0, nil
	}
	first := et.Posts[0]
	t := &store.Topic{ForumID: f.ID, CategoryID: cat, Title: clip(firstNonEmpty(et.Title, "Untitled"), maxTitle), Kind: firstNonEmpty(et.Kind, "discussion"), State: "visible", Closed: et.Closed,
		Pinned: et.Pinned, Tags: et.Tags, EventStart: et.EventStart, EventEnd: et.EventEnd, EventPlace: et.EventPlace, Source: source, SourceRef: clip(et.Key, 200), AuthorName: first.Author, CreatedAt: et.CreatedAt}
	if !oneOf(t.Kind, topicKinds) {
		t.Kind = "discussion"
	}
	p := &store.Post{Body: clip(firstNonEmpty(first.Body, "…"), maxBody), AuthorName: first.Author, State: "visible", CreatedAt: first.CreatedAt, Wiki: first.Wiki, Source: source}
	if first.Mine {
		t.UserID, p.UserID, p.AuthorName = u.ID, u.ID, s.actorName(u)
	}
	if err := s.store.CreateTopic(ctx, t, p); err != nil {
		return 0, err
	}
	n := 1
	var solved int64
	for i, ep := range et.Posts {
		if i == 0 {
			continue
		}
		q := &store.Post{TopicID: t.ID, ForumID: f.ID, ReplyTo: ep.ReplyTo, Body: clip(firstNonEmpty(ep.Body, "…"), maxBody), AuthorName: ep.Author, State: firstNonEmpty(ep.State, "visible"), Wiki: ep.Wiki, CreatedAt: ep.CreatedAt, Source: source}
		if !oneOf(q.State, []string{"visible", "hidden", "pending"}) {
			q.State = "visible"
		}
		if ep.Mine {
			q.UserID, q.AuthorName = u.ID, s.actorName(u)
		}
		if err := s.store.CreatePost(ctx, q); err != nil {
			return n, err
		}
		if ep.Poll != nil && len(ep.Poll.Options) >= 2 {
			poll := &store.Poll{PostID: q.ID, TopicID: t.ID, ForumID: f.ID, UserID: q.UserID, Question: ep.Poll.Question, Options: ep.Poll.Options, Multi: ep.Poll.Multi, Public: ep.Poll.Public, Closed: true}
			_ = s.store.SavePoll(ctx, poll)
		}
		if ep.Solution {
			solved = q.ID
		}
		n++
	}
	if first.Poll != nil && len(first.Poll.Options) >= 2 {
		_ = s.store.SavePoll(ctx, &store.Poll{PostID: p.ID, TopicID: t.ID, ForumID: f.ID, UserID: p.UserID, Question: first.Poll.Question, Options: first.Poll.Options, Multi: first.Poll.Multi, Public: first.Poll.Public, Closed: true})
	}
	if solved > 0 || et.Views > 0 {
		t.SolvedPostID = solved
		_ = s.store.UpdateTopic(ctx, t)
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE topics SET views = ? WHERE id = ?`, et.Views, t.ID)
	}
	s.store.RecountTopic(ctx, t.ID)
	return n, nil
}

// ---- bringing a Discourse forum over ----
//
// Discourse (and the many forums built on it) answers its public pages as
// JSON. This reads the categories and the newest topics with their posts and
// adds them here, authors by name. Running it again adds only what is new.

var reDiscourseTopic = regexp.MustCompile(`/t/(?:[^/]+/)?(\d+)`)

func publicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 { // 100.64/10 (Tailscale, CGNAT)
		return false
	}
	return true
}

// outsideClient fetches pages on the public internet only.
func (s *Server) outsideClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if publicIP(ip.IP) {
					return d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				}
			}
			return nil, errors.New("only forums on the public internet can be brought over")
		},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 40 * time.Second}
}

func (s *Server) getOutside(ctx context.Context, c *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", firstNonEmpty(s.cfg.UserAgent, "PocketForum/"+Version))
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("%s answered %d", req.URL.Host, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(v)
}

type dcPost struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Cooked     string `json:"cooked"`
	PostNumber int    `json:"post_number"`
	ReplyTo    int    `json:"reply_to_post_number"`
	CreatedAt  string `json:"created_at"`
	Accepted   bool   `json:"accepted_answer"`
	Hidden     bool   `json:"hidden"`
}

func (s *Server) handleImportDiscourse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := s.forumParam(w, r)
	if !ok {
		return
	}
	if !a.owner() || linkApp(ctx) != "" {
		writeErr(w, http.StatusForbidden, "only the forum's owner can bring topics in")
		return
	}
	var in struct {
		URL   string `json:"url"`
		Limit int    `json:"limit"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	base, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" {
		writeErr(w, http.StatusBadRequest, "paste the forum's address, like https://forum.example.com")
		return
	}
	root := base.Scheme + "://" + base.Host
	limit := clampInt(in.Limit, 1, 300)
	if in.Limit == 0 {
		limit = 50
	}
	c := s.outsideClient()
	var cats struct {
		CategoryList struct {
			Categories []struct {
				ID          int64  `json:"id"`
				Name        string `json:"name"`
				Slug        string `json:"slug"`
				Color       string `json:"color"`
				Description string `json:"description_text"`
			} `json:"categories"`
		} `json:"category_list"`
	}
	if err := s.getOutside(ctx, c, root+"/categories.json", &cats); err != nil {
		writeErr(w, http.StatusBadGateway, "that does not look like a Discourse forum: "+err.Error())
		return
	}
	catMap := map[int64]int64{}
	have, _ := s.store.ListCategories(ctx, a.f.ID, a.uid())
	bySlug := map[string]int64{}
	for _, x := range have {
		bySlug[x.Slug] = x.ID
	}
	for _, dc := range cats.CategoryList.Categories {
		sl := store.Slugify(firstNonEmpty(dc.Slug, dc.Name), "-")
		if sl == "" {
			continue
		}
		if id := bySlug[sl]; id > 0 {
			catMap[dc.ID] = id
			continue
		}
		color := ""
		if len(dc.Color) == 6 {
			color = "#" + strings.ToLower(dc.Color)
		}
		nc := &store.Category{ForumID: a.f.ID, Name: clip(dc.Name, 60), Slug: sl, About: clip(dc.Description, 300), Color: color, Starts: "all"}
		if err := s.store.SaveCategory(ctx, a.uid(), nc); err == nil {
			catMap[dc.ID], bySlug[sl] = nc.ID, nc.ID
		}
	}
	type dcTopic struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		CategoryID int64  `json:"category_id"`
		Tags       []any  `json:"tags"`
		Views      int    `json:"views"`
		Closed     bool   `json:"closed"`
		CreatedAt  string `json:"created_at"`
	}
	var list []dcTopic
	for page := 0; len(list) < limit && page < 30; page++ {
		var lt struct {
			TopicList struct {
				Topics []dcTopic `json:"topics"`
				More   string    `json:"more_topics_url"`
			} `json:"topic_list"`
		}
		if err := s.getOutside(ctx, c, fmt.Sprintf("%s/latest.json?page=%d", root, page), &lt); err != nil {
			break
		}
		list = append(list, lt.TopicList.Topics...)
		if lt.TopicList.More == "" || len(lt.TopicList.Topics) == 0 {
			break
		}
	}
	if len(list) > limit {
		list = list[:limit]
	}
	topics, posts, skipped := 0, 0, 0
	for _, dt := range list {
		key := fmt.Sprintf("%s/t/%d", base.Host, dt.ID)
		if _, err := s.store.TopicBySource(ctx, a.f.ID, "discourse", key); err == nil {
			skipped++
			continue
		}
		var full struct {
			Title      string `json:"title"`
			PostStream struct {
				Posts  []dcPost `json:"posts"`
				Stream []int64  `json:"stream"`
			} `json:"post_stream"`
		}
		if err := s.getOutside(ctx, c, fmt.Sprintf("%s/t/%d.json", root, dt.ID), &full); err != nil {
			continue
		}
		all := full.PostStream.Posts
		have := map[int64]bool{}
		for _, p := range all {
			have[p.ID] = true
		}
		var missing []int64
		for _, id := range full.PostStream.Stream {
			if !have[id] && len(missing) < 400 {
				missing = append(missing, id)
			}
		}
		for i := 0; i < len(missing); i += 20 {
			j := i + 20
			if j > len(missing) {
				j = len(missing)
			}
			q := url.Values{}
			for _, id := range missing[i:j] {
				q.Add("post_ids[]", strconv.FormatInt(id, 10))
			}
			var more struct {
				PostStream struct {
					Posts []dcPost `json:"posts"`
				} `json:"post_stream"`
			}
			if err := s.getOutside(ctx, c, fmt.Sprintf("%s/t/%d/posts.json?%s", root, dt.ID, q.Encode()), &more); err == nil {
				all = append(all, more.PostStream.Posts...)
			}
		}
		et := exportTopic{Key: key, Title: firstNonEmpty(full.Title, dt.Title), Kind: "discussion", Closed: dt.Closed, Views: dt.Views, CreatedAt: discourseTime(dt.CreatedAt)}
		for _, tg := range dt.Tags {
			switch v := tg.(type) {
			case string:
				et.Tags = append(et.Tags, v)
			case map[string]any:
				et.Tags = append(et.Tags, str(v, "name"))
			}
		}
		for _, p := range all {
			if p.Hidden {
				continue
			}
			body := htmlToMarkdown(p.Cooked, root)
			if strings.TrimSpace(body) == "" {
				continue
			}
			ep := exportPost{Author: firstNonEmpty(p.Name, p.Username), Number: p.PostNumber, ReplyTo: p.ReplyTo, Body: body, State: "visible", CreatedAt: discourseTime(p.CreatedAt), Solution: p.Accepted}
			if ep.Solution {
				et.Kind = "question"
			}
			et.Posts = append(et.Posts, ep)
		}
		n, err := s.importTopic(ctx, a.f, a.u, catMap[dt.CategoryID], et, "discourse")
		if err != nil {
			s.fail(w, err, "import")
			return
		}
		if n > 0 {
			topics++
			posts += n
		}
	}
	s.modLog(ctx, a, fmt.Sprintf("brought %d topics over from %s", topics, base.Host), "forum", a.f.ID, root)
	text := fmt.Sprintf("Brought %d %s with %d %s over from %s", topics, plural(topics, "topic", "topics"), posts, plural(posts, "post", "posts"), base.Host)
	if skipped > 0 {
		text += fmt.Sprintf(" (%d were already here)", skipped)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "topics": topics, "posts": posts, "skipped": skipped, "text": text})
}

func discourseTime(v string) string {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return store.Now()
}

var (
	reDCAside   = regexp.MustCompile(`(?s)<aside[^>]*class="quote[^"]*"[^>]*data-username="([^"]*)"(?:[^>]*data-post="(\d+)")?[^>]*>.*?<blockquote>(.*?)</blockquote>\s*</aside>`)
	reDCOnebox  = regexp.MustCompile(`(?s)<aside[^>]*class="onebox[^"]*"[^>]*data-onebox-src="([^"]*)"[^>]*>.*?</aside>`)
	reDCPre     = regexp.MustCompile(`(?s)<pre[^>]*><code[^>]*>(.*?)</code></pre>`)
	reDCImg     = regexp.MustCompile(`<img[^>]*src="([^"]+)"[^>]*?(?:alt="([^"]*)")?[^>]*>`)
	reDCLink    = regexp.MustCompile(`(?s)<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	reDCTag     = regexp.MustCompile(`<[^>]+>`)
	reDCBlank   = regexp.MustCompile(`\n{3,}`)
	reDCLightbx = regexp.MustCompile(`(?s)<div class="lightbox-wrapper">.*?(<img[^>]*>).*?</div>`)
)

// htmlToMarkdown turns a Discourse post's HTML back into Markdown.
func htmlToMarkdown(h, root string) string {
	abs := func(u string) string {
		u = html.UnescapeString(u)
		if strings.HasPrefix(u, "//") {
			return "https:" + u
		}
		if strings.HasPrefix(u, "/") {
			return root + u
		}
		return u
	}
	h = reDCLightbx.ReplaceAllString(h, "$1")
	h = reDCAside.ReplaceAllStringFunc(h, func(m string) string {
		p := reDCAside.FindStringSubmatch(m)
		inner := strings.TrimSpace(htmlToMarkdown(p[3], root))
		head := "[quote=" + store.Slugify(p[1], "_")
		if p[2] != "" {
			head += " #" + p[2]
		}
		return "\n\n" + head + "]\n" + inner + "\n[/quote]\n\n"
	})
	h = reDCOnebox.ReplaceAllString(h, "\n\n$1\n\n")
	h = reDCPre.ReplaceAllStringFunc(h, func(m string) string {
		return "\n\n```\n" + html.UnescapeString(reDCPre.FindStringSubmatch(m)[1]) + "\n```\n\n"
	})
	h = reDCImg.ReplaceAllStringFunc(h, func(m string) string {
		p := reDCImg.FindStringSubmatch(m)
		if strings.Contains(m, "class=\"emoji") {
			return html.UnescapeString(firstNonEmpty(p[2], ""))
		}
		return "![" + html.UnescapeString(p[2]) + "](" + abs(p[1]) + ")"
	})
	h = reDCLink.ReplaceAllStringFunc(h, func(m string) string {
		p := reDCLink.FindStringSubmatch(m)
		text := strings.TrimSpace(reDCTag.ReplaceAllString(p[2], ""))
		if strings.Contains(m, `class="mention"`) {
			return text
		}
		if strings.HasPrefix(text, "![") {
			return text
		}
		return "[" + firstNonEmpty(text, abs(p[1])) + "](" + abs(p[1]) + ")"
	})
	r := strings.NewReplacer("<strong>", "**", "</strong>", "**", "<b>", "**", "</b>", "**", "<em>", "*", "</em>", "*", "<i>", "*", "</i>", "*",
		"<code>", "`", "</code>", "`", "<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n\n", "<p>", "", "<li>", "- ", "</li>", "\n",
		"<blockquote>", "\n> ", "</blockquote>", "\n\n", "<hr>", "\n\n---\n\n", "<h1>", "\n## ", "<h2>", "\n## ", "<h3>", "\n### ", "<h4>", "\n### ",
		"</h1>", "\n\n", "</h2>", "\n\n", "</h3>", "\n\n", "</h4>", "\n\n")
	h = r.Replace(h)
	h = reDCTag.ReplaceAllString(h, "")
	h = html.UnescapeString(h)
	return strings.TrimSpace(reDCBlank.ReplaceAllString(h, "\n\n"))
}

var _ = reDiscourseTopic
