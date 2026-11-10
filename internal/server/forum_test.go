package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// makeForum is Alice's forum with the given visibility; it returns its slug.
func (a *app) makeForum(name, vis, join string) string {
	a.t.Helper()
	m := a.ok(a.alice, "POST", "/api/v1/forums", map[string]any{"name": name, "visibility": vis, "join_policy": join, "template": "community"})
	return obj(m, "forum")["slug"].(string)
}

func (a *app) topic(auth, forum, title, body string) (int64, map[string]any) {
	a.t.Helper()
	m := a.ok(auth, "POST", "/api/v1/topics", map[string]any{"forum": forum, "title": title, "body": body})
	return int64(num(obj(m, "topic"), "id")), m
}

// A private forum is invisible to everyone outside it — the box's admin
// included when it is not theirs — and members only see their own forums.
func TestPrivateForumIsolation(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Family", "private", "invite")
	tid, _ := a.topic(a.alice, slug, "Sunday lunch", "Who is coming on Sunday?")

	a.expect(404, a.bob, "GET", "/api/v1/forums/"+slug, nil)
	a.expect(404, a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)
	a.expect(404, a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Me!"})
	a.expect(404, a.bob, "POST", "/api/v1/forums/"+slug+"/join", nil)
	a.expect(404, a.bob, "GET", "/api/v1/forums/"+slug+"/members", nil)
	a.expect(404, a.bob, "POST", "/api/v1/topics", map[string]any{"forum": slug, "title": "Sneaky", "body": "in"})
	if hits := a.ok(a.bob, "GET", "/api/v1/search?q=lunch", nil)["hits"].([]any); len(hits) != 0 {
		t.Fatalf("Bob finds a private post: %v", hits)
	}
	if l := a.ok(a.bob, "GET", "/api/v1/topics", nil)["topics"].([]any); len(l) != 0 {
		t.Fatalf("Bob lists a private topic: %v", l)
	}
	if code, _, _ := a.call("", "GET", "/api/v1/public/forums/"+slug, nil); code != 404 {
		t.Fatalf("a private forum answers publicly: %d", code)
	}

	// Bob's own private forum is closed to Alice, the box's admin.
	m := a.ok(a.bob, "POST", "/api/v1/forums", map[string]any{"name": "Bob's", "visibility": "private"})
	bslug := obj(m, "forum")["slug"].(string)
	btid, _ := a.topic(a.bob, bslug, "My diary", "Private thoughts here")
	a.expect(404, a.alice, "GET", "/api/v1/forums/"+bslug, nil)
	a.expect(404, a.alice, "GET", fmt.Sprintf("/api/v1/topics/%d", btid), nil)
	a.expect(404, a.alice, "PUT", "/api/v1/forums/"+bslug, map[string]any{"name": "Mine now"})
	a.expect(404, a.alice, "DELETE", "/api/v1/forums/"+bslug, nil)

	// with an invite Bob is in, and sees it all
	inv := a.ok(a.alice, "POST", "/api/v1/forums/"+slug+"/invites", map[string]any{})
	tok := obj(inv, "invite")["token"].(string)
	a.ok(a.bob, "POST", "/api/v1/invites/"+tok+"/accept", nil)
	a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)
	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Me!"})
	// a member is not a moderator
	a.expect(403, a.bob, "PUT", "/api/v1/forums/"+slug, map[string]any{"name": "Taken"})
	a.expect(403, a.bob, "GET", "/api/v1/forums/"+slug+"/invites", nil)
	a.expect(403, a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/status", tid), map[string]any{"pinned": 2})
	// and Alice's post is not Bob's to change
	first := a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)["posts"].([]any)[0].(map[string]any)
	a.expect(403, a.bob, "PUT", "/api/v1/posts/"+id(first), map[string]any{"body": "changed"})
	a.expect(403, a.bob, "DELETE", "/api/v1/posts/"+id(first), nil)
}

// A box forum: anyone with an account reads; posting needs joining; a
// ban stops posting; an approval forum holds people until let in.
func TestJoiningAndModeration(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Street", "box", "open")
	tid, _ := a.topic(a.alice, slug, "Ladder to lend", "Who has a ladder?")
	a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)
	a.expect(400, a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "I do"})
	a.ok(a.bob, "POST", "/api/v1/forums/"+slug+"/join", nil)
	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "I do, ring no. 12"})

	bob := a.ids["Bob"]
	a.ok(a.alice, "PUT", fmt.Sprintf("/api/v1/forums/%s/members/%d", slug, bob), map[string]any{"status": "banned", "ban_days": 3, "ban_reason": "spam"})
	a.expect(400, a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "again"})
	a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil) // still reads a box forum
	a.ok(a.alice, "PUT", fmt.Sprintf("/api/v1/forums/%s/members/%d", slug, bob), map[string]any{"status": "active"})

	// held words hold a post; blocked words refuse it
	a.ok(a.alice, "PUT", "/api/v1/forums/"+slug, map[string]any{"held_words": []string{"casino"}, "blocked_words": []string{"idiot"}})
	m := a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Visit my casino"})
	if m["state"] != "pending" {
		t.Fatalf("a held word should hold the post: %v", m)
	}
	a.expect(400, a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "you idiot"})
	rv := a.ok(a.alice, "GET", "/api/v1/review", nil)
	if len(rv["held"].([]any)) != 1 {
		t.Fatalf("the held post should wait in review: %v", rv["held"])
	}
	pid := int64(num(obj(m, "post"), "id"))
	a.expect(403, a.bob, "POST", fmt.Sprintf("/api/v1/posts/%d/review", pid), map[string]any{"action": "approve"}) // his own held post: he sees it, but only a moderator approves
	a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/posts/%d/review", pid), map[string]any{"action": "approve"})

	// an approval forum
	ap := a.makeForum("Club", "box", "approval")
	a.ok(a.bob, "POST", "/api/v1/forums/"+ap+"/join", map[string]any{"note": "hi"})
	a.expect(400, a.bob, "POST", "/api/v1/topics", map[string]any{"forum": ap, "title": "Can I post", "body": "please"})
	a.ok(a.alice, "PUT", fmt.Sprintf("/api/v1/forums/%s/members/%d", ap, bob), map[string]any{"approve": true})
	a.topic(a.bob, ap, "Hello club", "Glad to be here")
}

func TestRepliesReadsAndNotifications(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Help", "box", "open")
	a.ok(a.bob, "POST", "/api/v1/forums/"+slug+"/join", nil)
	tid, _ := a.topic(a.bob, slug, "How do I back up?", "Is it safe to copy the data folder?")
	alice := a.ok(a.alice, "GET", "/api/v1/home", nil)["profile"].(map[string]any)["handle"].(string)
	a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Use the backup command.", "reply_to": 1})
	bobHandle := a.ok(a.bob, "GET", "/api/v1/home", nil)["profile"].(map[string]any)["handle"].(string)
	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Thanks @" + alice + "!"})

	// notifications are written in the background
	waitFor(t, func() bool {
		return len(a.ok(a.bob, "GET", "/api/v1/notifications", nil)["notifications"].([]any)) > 0 &&
			len(a.ok(a.alice, "GET", "/api/v1/notifications", nil)["notifications"].([]any)) > 0
	})
	kinds := func(auth string) string {
		var out []string
		for _, n := range a.ok(auth, "GET", "/api/v1/notifications", nil)["notifications"].([]any) {
			out = append(out, n.(map[string]any)["kind"].(string))
		}
		return strings.Join(out, ",")
	}
	if !strings.Contains(kinds(a.bob), "reply") || !strings.Contains(kinds(a.alice), "mention") {
		t.Fatalf("bob: %s, alice: %s (handle %s/%s)", kinds(a.bob), kinds(a.alice), alice, bobHandle)
	}

	// unread: Alice read up to her post; Bob's reply is new for her
	doc := a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)
	if u := num(obj(doc, "topic"), "unread"); u != 1 {
		t.Fatalf("Alice should have 1 unread post, has %v", u)
	}
	a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/topics/%d/read", tid), map[string]any{"number": 3})
	if u := num(obj(a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil), "topic"), "unread"); u != 0 {
		t.Fatalf("read → unread %v", u)
	}

	// the person who asked marks the solution; it counts for the answerer
	posts := a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)["posts"].([]any)
	ans := posts[1].(map[string]any)
	a.expect(400, a.alice, "POST", fmt.Sprintf("/api/v1/topics/%d/solve", tid), map[string]any{"post_id": num(ans, "id")}) // a discussion has no solution
}

func TestQuestionsReactionsPolls(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Q&A", "box", "open")
	a.ok(a.bob, "POST", "/api/v1/forums/"+slug+"/join", nil)
	m := a.ok(a.bob, "POST", "/api/v1/topics", map[string]any{"forum": slug, "title": "Which day suits?", "body": "Pick one", "kind": "question",
		"poll": map[string]any{"question": "Day?", "options": []string{"Monday", "Tuesday"}}})
	tid := int64(num(obj(m, "topic"), "id"))
	reply := a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "Tuesday works for everyone."})
	rid := int64(num(obj(reply, "post"), "id"))
	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/solve", tid), map[string]any{"post_id": rid}) // the person who asked

	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/posts/%d/react", rid), map[string]any{"emoji": "❤️"})
	a.expect(400, a.bob, "POST", fmt.Sprintf("/api/v1/posts/%d/react", rid), map[string]any{"emoji": "🦄"})
	doc := a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)
	posts := doc["posts"].([]any)
	if num(posts[1].(map[string]any), "likes_n") != 1 {
		t.Fatalf("the reaction should count: %v", posts[1])
	}
	poll := obj(posts[0].(map[string]any), "poll")
	a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/polls/%d/vote", int64(num(poll, "id"))), map[string]any{"choices": []int{1}})
	p := a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil)["posts"].([]any)[0].(map[string]any)
	if obj(p, "poll")["counts"].([]any)[1].(float64) != 1 {
		t.Fatalf("vote not counted: %v", obj(p, "poll"))
	}
	if obj(a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/topics/%d", tid), nil), "topic")["solved_post_id"].(float64) != float64(rid) {
		t.Fatal("not solved")
	}
}

func TestPublicForumAndGuests(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Support", "public", "open")
	a.ok(a.alice, "PUT", "/api/v1/forums/"+slug, map[string]any{"guest_posts": true})
	tid, _ := a.topic(a.alice, slug, "Welcome", "Ask anything here.")
	a.expect(200, "", "GET", "/api/v1/public/forums/"+slug, nil)
	a.expect(200, "", "GET", fmt.Sprintf("/api/v1/public/topics/%d", tid), nil)
	m := a.expect(201, "", "POST", fmt.Sprintf("/api/v1/public/topics/%d/posts", tid), map[string]any{"body": "Does it run on a Pi?", "guest_name": "Nina"})
	if m["state"] != "pending" {
		t.Fatalf("a guest post waits: %v", m)
	}
	doc := a.ok("", "GET", fmt.Sprintf("/api/v1/public/topics/%d", tid), nil)
	if len(doc["posts"].([]any)) != 1 {
		t.Fatal("the guest's held post must not show publicly")
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/feeds/"+slug+".rss", nil)
	a.s.Router().ServeHTTP(res, req)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "<title>Welcome</title>") {
		t.Fatalf("feed: %d %s", res.Code, res.Body.String())
	}
}

// Another Pocket app acting for a person (a pcx_ link token) gets only the
// routes of its scopes, and may change only what it wrote itself.
func TestLinkScopes(t *testing.T) {
	type c struct {
		method, path string
		scopes       []string
		ok           bool
	}
	for _, x := range []c{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/summary", []string{"forum:read"}, true},
		{"GET", "/api/v1/topics", []string{"forum:read"}, true},
		{"GET", "/api/v1/topics/3", []string{"forum:read"}, true},
		{"GET", "/api/v1/day/2026-11-10", []string{"forum:read"}, true},
		{"POST", "/api/v1/topics", []string{"forum:read"}, false},
		{"POST", "/api/v1/topics", []string{"forum:write"}, true},
		{"POST", "/api/v1/topics/3/posts", []string{"forum:write"}, true},
		{"PUT", "/api/v1/posts/9", []string{"forum:write"}, true},
		{"PUT", "/api/v1/forums/x", []string{"forum:write", "forum:read"}, false},
		{"POST", "/api/v1/topics/3/status", []string{"forum:write"}, false},
		{"GET", "/api/v1/review", []string{"forum:read"}, false},
		{"GET", "/api/v1/settings", []string{"forum:read", "forum:write"}, false},
		{"POST", "/api/v1/forums", []string{"forum:write"}, false},
	} {
		r := httptest.NewRequest(x.method, x.path, nil)
		if got := linkAllows(x.scopes, r); got != x.ok {
			t.Errorf("%s %s with %v: %v, want %v", x.method, x.path, x.scopes, got, x.ok)
		}
	}
	a := newApp(t, nil)
	slug := a.makeForum("Blogged", "private", "invite")
	ctx := withLinkApp(context.Background(), "blog")
	ctx = context.WithValue(ctx, userKey, &store.User{ID: a.ids["Alice"], Email: "alice@example.com", Name: "Alice"})
	ac, _ := a.s.forumAccess(ctx, userFrom(ctx), slug)
	res, err := a.s.createTopic(ctx, ac, topicIn{Title: "Discuss: my post", Body: "What do you think?", SourceRef: "post:7"}, false)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := a.s.createTopic(ctx, ac, topicIn{Title: "Discuss: my post", Body: "What do you think?", SourceRef: "post:7"}, false)
	if again["existing"] != true {
		t.Fatal("the same source should find the same topic")
	}
	tp := res["topic"].(*store.Topic)
	if tp.Source != "blog" {
		t.Fatalf("source: %q", tp.Source)
	}
	if !mayChange(ctx, "blog") || mayChange(ctx, "") || mayChange(withLinkApp(context.Background(), "chat"), "blog") {
		t.Fatal("a link may change only its own app's rows")
	}
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	slug := a.makeForum("Book club", "private", "invite")
	tid, _ := a.topic(a.alice, slug, "Next book", "Suggestions please")
	a.ok(a.alice, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "The Hobbit"})
	code, doc, _ := a.call(a.alice, "GET", "/api/v1/export.json", nil)
	if code != 200 || len(doc["forums"].([]any)) != 1 {
		t.Fatalf("export: %d %v", code, doc)
	}
	b, _ := json.Marshal(doc)
	// into Bob's account: once makes the forum, twice adds nothing
	var ed exportDoc
	_ = json.Unmarshal(b, &ed)
	bob, _ := a.st.GetUser(context.Background(), a.ids["Bob"])
	r1, err := a.s.importDoc(context.Background(), bob, &ed)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.s.importDoc(context.Background(), bob, &ed)
	if err != nil {
		t.Fatal(err)
	}
	if r1["topics"] != 1 || r1["posts"] != 2 || r2["topics"] != 0 {
		t.Fatalf("first %v, second %v", r1, r2)
	}
	// Bob's copy is his; Alice's original stays hers
	forums := a.ok(a.bob, "GET", "/api/v1/forums", nil)["forums"].([]any)
	if len(forums) != 1 || forums[0].(map[string]any)["role"] != "owner" || forums[0].(map[string]any)["slug"] == slug {
		t.Fatalf("Bob's forums: %v", forums)
	}
}

func TestDiscourseHTML(t *testing.T) {
	h := `<aside class="quote" data-username="sam" data-post="2"><div class="title">sam:</div><blockquote><p>old words</p></blockquote></aside><p>Hi <a class="mention" href="/u/bob">@bob</a>, see <a href="/t/x/9">this</a> and <strong>bold</strong></p><pre><code>go test</code></pre>`
	md := htmlToMarkdown(h, "https://f.example")
	for _, want := range []string{"[quote=sam #2]", "old words", "[/quote]", "@bob", "[this](https://f.example/t/x/9)", "**bold**", "```\ngo test\n```"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if !publicIP([]byte{8, 8, 8, 8}) || publicIP([]byte{100, 100, 1, 1}) || publicIP([]byte{192, 168, 1, 2}) || publicIP([]byte{127, 0, 0, 1}) {
		t.Error("public address check")
	}
}

func TestMentionsAndText(t *testing.T) {
	got := mentions("Hi @emir and @Lejla.\n[quote=sam #2]@notme[/quote] `@code` email a@b.com")
	if strings.Join(got, ",") != "emir,lejla" {
		t.Fatalf("mentions: %v", got)
	}
	if q := quoted("[quote=sam #2]x[/quote]"); q["sam"] != 2 {
		t.Fatalf("quoted: %v", q)
	}
	if w := containsWord("Buy CHEAP pills now", []string{"cheap pills"}); w != "cheap pills" {
		t.Fatalf("containsWord: %q", w)
	}
	if containsWord("scheapest", []string{"cheap"}) != "" {
		t.Fatal("whole words only")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if ok() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("timed out")
}
