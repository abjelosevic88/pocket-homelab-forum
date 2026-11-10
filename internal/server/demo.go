package server

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// seedDemo fills the owner's account with a few lived-in forums (DEMO_DATA):
// a public help forum, a private family one, a club with events, and one of
// a neighbour's to discover. Everyone but the owner is a demo account
// without a password.

type demoReply struct {
	who     int
	body    string
	replyTo int
	solves  bool
	likes   int
}

type demoTopic struct {
	cat     string
	kind    string
	who     int
	title   string
	body    string
	tags    []string
	days    float64
	pinned  int
	closed  bool
	event   string
	place   string
	poll    *store.Poll
	replies []demoReply
	read    int // how far the owner read: -1 never opened, 0 everything
}

var demoPeople = []struct{ name, email, color string }{
	{"", "", ""},
	{"Lejla Hadžić", "lejla@demo.local", "#c2410c"},
	{"Marko Petrović", "marko@demo.local", "#0f766e"},
	{"Ana Jurić", "ana@demo.local", "#7c3aed"},
	{"Dino Begić", "dino@demo.local", "#2563eb"},
	{"Sara Novak", "sara@demo.local", "#be123c"},
	{"Ivan Horvat", "ivan@demo.local", "#15803d"},
	{"Amra Delić", "amra@demo.local", "#a16207"},
}

func (s *Server) seedDemo(ctx context.Context, owner int64) error {
	ctx = context.WithoutCancel(ctx)
	if _, _, done := s.store.CacheGet(ctx, "demo:seeded"); done {
		return nil
	}
	if n, _ := s.store.MyForums(ctx, owner); len(n) > 0 {
		return nil
	}
	_ = s.store.CachePut(ctx, "demo:seeded", "1")
	rnd := rand.New(rand.NewSource(42))
	ou, err := s.store.GetUser(ctx, owner)
	if err != nil {
		return err
	}
	if ou.Name == "" || ou.Name == "Admin" {
		ou.Name = "Emir Kovač"
		_ = s.store.UpdateUser(ctx, ou)
	}
	ids := []int64{owner}
	for _, p := range demoPeople[1:] {
		u, err := s.store.GetUserByEmail(ctx, p.email)
		if err != nil {
			u = &store.User{Email: p.email, Name: p.name, Role: "member"}
			if err := s.store.CreateUser(ctx, u); err != nil {
				return err
			}
		}
		ids = append(ids, u.ID)
		if pr, err := s.store.EnsureProfile(ctx, u.ID); err == nil {
			pr.Color = p.color
			_ = s.store.UpdateProfile(ctx, pr)
		}
	}
	if pr, err := s.store.EnsureProfile(ctx, owner); err == nil {
		pr.Bio, pr.Color = "Runs the Pocket apps at home. Asks too many questions about backups.", "#4361c2"
		_ = s.store.UpdateProfile(ctx, pr)
	}
	name := func(i int) string {
		if i == 0 {
			return ou.Name
		}
		return demoPeople[i].name
	}
	now := time.Now()
	ago := func(days float64) string {
		return now.Add(-time.Duration(days * 24 * float64(time.Hour))).UTC().Format(time.RFC3339Nano)
	}

	mkForum := func(tplID, nameF, slug, about, emoji, color, vis, join string, ownerIdx int, guests bool) (*store.Forum, map[string]int64, error) {
		tpl := templateByID(tplID)
		f := &store.Forum{UserID: ids[ownerIdx], Slug: s.store.FreeForumSlug(ctx, slug, 0), Name: nameF, About: about, Emoji: emoji, Color: color, Visibility: vis, JoinPolicy: join,
			GuestPosts: guests, Rules: tpl.Rules, Welcome: tpl.Welcome, Reactions: []string{}}
		if err := s.store.CreateForum(ctx, f, tpl.Categories); err != nil {
			return nil, nil, err
		}
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE forums SET created_at = ? WHERE id = ?`, ago(120), f.ID)
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE members SET joined_at = ? WHERE forum_id = ?`, ago(118), f.ID)
		cats, _ := s.store.ListCategories(ctx, f.ID, owner)
		m := map[string]int64{}
		for _, c := range cats {
			m[c.Name] = c.ID
		}
		return f, m, nil
	}
	addMembers := func(f *store.Forum, who []int, mods ...int) {
		for _, i := range who {
			if ids[i] == f.UserID {
				continue
			}
			role := "member"
			for _, m := range mods {
				if m == i {
					role = "moderator"
				}
			}
			_ = s.store.AddMember(ctx, &store.Member{ForumID: f.ID, UserID: ids[i], Role: role, Status: "active", Level: "normal"})
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE members SET joined_at = ? WHERE forum_id = ? AND user_id = ?`, ago(110-float64(i)*6), f.ID, ids[i])
		}
	}
	seed := func(f *store.Forum, cats map[string]int64, topics []demoTopic) error {
		reacts := f.ReactionList()
		for _, dt := range topics {
			at := dt.days
			t := &store.Topic{ForumID: f.ID, CategoryID: cats[dt.cat], UserID: ids[dt.who], AuthorName: name(dt.who), Title: dt.title, Kind: firstNonEmpty(dt.kind, "discussion"),
				State: "visible", Pinned: dt.pinned, Tags: dt.tags, CreatedAt: ago(at)}
			if dt.event != "" {
				t.Kind, t.EventStart, t.EventPlace = "event", dt.event, dt.place
			}
			p := &store.Post{UserID: ids[dt.who], AuthorName: name(dt.who), Body: dt.body, State: "visible", CreatedAt: ago(at)}
			if err := s.store.CreateTopic(ctx, t, p); err != nil {
				return err
			}
			if dt.poll != nil {
				poll := *dt.poll
				poll.PostID, poll.TopicID, poll.ForumID, poll.UserID = p.ID, t.ID, f.ID, ids[dt.who]
				if err := s.store.SavePoll(ctx, &poll); err == nil {
					for i := 1; i < len(ids); i++ {
						_ = s.store.Vote(ctx, poll.ID, ids[i], []int{rnd.Intn(len(poll.Options))})
					}
				}
			}
			_ = s.store.SetTopicLevel(ctx, t.ID, ids[dt.who], "watching")
			step := at / float64(len(dt.replies)+1)
			var solved int64
			for i, r := range dt.replies {
				at -= step * (0.5 + rnd.Float64())
				if at < 0.02 {
					at = 0.02 + rnd.Float64()*0.05
				}
				q := &store.Post{TopicID: t.ID, ForumID: f.ID, UserID: ids[r.who], AuthorName: name(r.who), Body: r.body, ReplyTo: r.replyTo, State: "visible", CreatedAt: ago(at)}
				if err := s.store.CreatePost(ctx, q); err != nil {
					return err
				}
				if r.solves {
					solved = q.ID
				}
				for k := 0; k < r.likes; k++ {
					who := 1 + (i+k*3)%(len(ids)-1)
					if ids[who] != q.UserID {
						_, _ = s.store.ToggleReaction(ctx, q, ids[who], reacts[(k+i)%3])
					}
				}
				s.store.MarkRead(ctx, t.ID, ids[r.who], q.Number)
			}
			for k := 0; k < 2+rnd.Intn(5); k++ {
				who := 1 + rnd.Intn(len(ids)-1)
				if ids[who] != p.UserID {
					_, _ = s.store.ToggleReaction(ctx, p, ids[who], reacts[rnd.Intn(2)])
				}
			}
			if solved > 0 {
				t.SolvedPostID = solved
			}
			t.Closed = dt.closed
			_ = s.store.UpdateTopic(ctx, t)
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE topics SET views = ? WHERE id = ?`, 12+rnd.Intn(240)+len(dt.replies)*9, t.ID)
			switch {
			case dt.read == 0:
				s.store.MarkRead(ctx, t.ID, owner, len(dt.replies)+1)
			case dt.read > 0:
				s.store.MarkRead(ctx, t.ID, owner, dt.read)
			}
			s.store.RecountTopic(ctx, t.ID)
			for _, uid := range ids {
				s.store.RecountMember(ctx, f.ID, uid)
			}
		}
		return nil
	}

	// 1. A public help forum for the Pocket apps.
	help, hc, err := mkForum("support", "Pocket Homelab", "pocket-homelab", "Help, guides and ideas for the Pocket apps: one Go binary, one SQLite file, one volume.", "🏠", "#4361c2", "public", "open", 0, true)
	if err != nil {
		return err
	}
	addMembers(help, []int{1, 2, 3, 4, 5, 6, 7}, 2)
	if err := seed(help, hc, []demoTopic{
		{cat: "Announcements", who: 0, pinned: 2, days: 20, read: 0, tags: []string{"release"}, title: "Pocket Forum is here: forums for your family, club or community",
			body: "Hello everyone 👋\n\nThis forum runs on **Pocket Forum**, the newest of the Pocket apps. What it does:\n\n- Categories, topics and replies with quotes, @mentions and reactions\n- Questions that get an **accepted answer**\n- Events where you can say whether you are coming\n- Polls, pictures and files\n- Everything searchable, and new / unread tracked for you\n\nAsk anything in **Help** — and please mark the reply that solved it, so the next person finds it.",
			replies: []demoReply{{who: 1, body: "Finally a place where answers don't scroll away like in the group chat. Thank you!", likes: 4}, {who: 4, body: "The unread tracking alone is worth it. Is there an RSS feed for announcements?", likes: 1},
				{who: 0, replyTo: 3, body: "Yes — public forums have one: open the forum's *About* tab and copy the feed link. You can also follow just one category.", likes: 2}}},
		{cat: "Help", kind: "question", who: 4, days: 14, read: 0, tags: []string{"backups"}, title: "How do I back up a Pocket app without stopping it?",
			body: "I run about twenty Pocket apps in Docker. Is it safe to copy the `data/` folder while the container is running, or do I need to stop it first?",
			replies: []demoReply{{who: 2, body: "Don't copy the live `.db` file — you can catch it half-written. Every app has a `backup` command that writes a consistent copy:\n\n```\ndocker exec pocket-forum /app/pocket-forum backup -o /data/backups/now.db\n```\n\nIt uses SQLite's `VACUUM INTO`, so the app keeps running.", solves: true, likes: 6},
				{who: 4, replyTo: 2, body: "That's exactly what I needed, thanks Marko. Marked as the solution."},
				{who: 6, body: "And if you use Pocket Backups, add the folder but leave the live database out — it takes the copies from the backups folder."}}},
		{cat: "Help", kind: "question", who: 5, days: 9, read: 2, tags: []string{"sign-in", "core"}, title: "\"Sign in with Pocket Core\" button is missing",
			body: "On my new install the login page only shows email and password. The other apps on the same box have the Pocket Core button. What am I missing?",
			replies: []demoReply{{who: 3, body: "Is `CORE_URL` set for that container? Without it the app runs on its own and the button stays hidden."},
				{who: 5, replyTo: 2, body: "It is set, and `/healthz` shows the Core outbox…"},
				{who: 2, body: "Then check `BASE_URL`: Core registers the sign-in callback at `BASE_URL/api/v1/auth/oidc/callback`. If it is still `http://localhost:8080`, Core refuses it.", solves: true, likes: 3},
				{who: 5, body: "That was it — BASE_URL was wrong. Working now 🎉", likes: 2}}},
		{cat: "Help", kind: "question", who: 7, days: 3, read: -1, tags: []string{"notifications"}, title: "Phone notifications arrive twice",
			body:    "Since yesterday every mention pings my phone twice, a few seconds apart. Apprise → ntfy. Anyone seen this?",
			replies: []demoReply{{who: 1, body: "Do you have the same ntfy topic in two Apprise routes? I had that after copying a config."}}},
		{cat: "Guides", who: 2, days: 30, read: 0, tags: []string{"caddy", "guide"}, title: "Guide: put a Pocket app behind Caddy in five minutes",
			body:    "## What you need\n\n- Caddy running in Docker\n- A DNS name pointing at your box\n\n## The block\n\n```\nforum.example.org {\n\treverse_proxy host.docker.internal:8267 {\n\t\tflush_interval -1\n\t}\n}\n```\n\n`flush_interval -1` matters: it lets live updates through straight away.\n\n## Check\n\nOpen `/healthz` — you should see `{\"status\":\"ok\"}`.",
			replies: []demoReply{{who: 6, body: "Bookmarked. The flush_interval line was my missing piece for Chat too.", likes: 3}, {who: 3, body: "Could you add the `request_body` limit for apps that take big uploads?"}, {who: 2, replyTo: 3, body: "Good idea, I'll add a section.", likes: 1}}},
		{cat: "Bug reports", who: 6, days: 6, read: 1, tags: []string{"calendar"}, title: "Calendar week view: events after 23:00 drop off the bottom",
			body:    "**What I did:** added an event 23:30–00:30.\n\n**What happened:** it doesn't show in the week view; the month view has it.\n\n**Expected:** a block at the bottom of the day.",
			replies: []demoReply{{who: 0, body: "Confirmed here too. I turned it into an issue for the team.", likes: 2}, {who: 6, body: "Thanks!"}}},
		{cat: "Feature requests", who: 1, days: 11, read: 0, tags: []string{"ideas"}, title: "Which app should get offline mode first?",
			body:    "Offline support is coming to one app first. Which would help you most?",
			poll:    &store.Poll{Question: "Offline mode first in…", Options: []string{"Notes", "Tasks", "Kitchen (shopping list)", "Forum"}, Public: false},
			replies: []demoReply{{who: 3, body: "Shopping list, no question — the supermarket basement has no signal 😅", likes: 5}, {who: 4, body: "Notes for me."}, {who: 5, body: "+1 for the shopping list.", likes: 1}}},
		{cat: "Feature requests", who: 3, days: 1.5, read: -1, tags: []string{"ideas", "forum"}, title: "Let us follow a single tag",
			body:    "I would love to watch only topics tagged `backups`, wherever they are posted.",
			replies: []demoReply{{who: 2, body: "Yes please. For now you can search `#backups` and bookmark the search."}}},
	}); err != nil {
		return err
	}
	// a guest's question waiting for approval, and a flag
	if t, err := s.store.ListTopics(ctx, store.TopicQuery{ForumIDs: []int64{help.ID}, Limit: 3}); err == nil && len(t) > 1 {
		guest := &store.Post{TopicID: t[1].ID, ForumID: help.ID, AuthorName: "Nina (guest)", GuestEmail: "nina@example.com", Body: "Does this also work with Podman instead of Docker?", State: "pending", CreatedAt: ago(0.3)}
		_ = s.store.CreatePost(ctx, guest)
		s.tellMods(ctx, help, store.Notification{Kind: "pending", TopicID: t[1].ID, PostID: guest.ID, Number: guest.Number, ActorName: guest.AuthorName, Title: t[1].Title, Excerpt: guest.Body}, 0)
	}
	spam := &store.Topic{ForumID: help.ID, CategoryID: hc["Help"], UserID: ids[7], AuthorName: name(7), Title: "Cheap watches, best prices", Kind: "discussion", State: "visible", CreatedAt: ago(0.2)}
	sp := &store.Post{UserID: ids[7], AuthorName: name(7), Body: "Visit my shop for the best deals on watches!!!", State: "visible", CreatedAt: ago(0.2)}
	if err := s.store.CreateTopic(ctx, spam, sp); err == nil {
		_ = s.store.AddFlag(ctx, &store.Flag{ForumID: help.ID, PostID: sp.ID, UserID: ids[4], Reason: "spam"})
		_ = s.store.AddFlag(ctx, &store.Flag{ForumID: help.ID, PostID: sp.ID, UserID: ids[5], Reason: "spam", Note: "Account posted this in three categories"})
	}

	// 2. The family.
	fam, fc, err := mkForum("family", "Kovač family", "kovac-family", "News, plans, recipes and old photos — just us.", "🏡", "#a83b6c", "private", "invite", 0, false)
	if err != nil {
		return err
	}
	addMembers(fam, []int{1, 3, 7}, 1)
	if err := seed(fam, fc, []demoTopic{
		{cat: "Plans", who: 1, days: 12, read: 2, event: now.AddDate(0, 0, 9).Format("2006-01-02") + "T13:00", place: "Nana's garden, Ilidža", title: "Nana's 80th birthday lunch",
			body: "It's happening! Saturday lunch at Nana's. Please say whether you are coming so we know how many chairs to borrow.\n\nI'll make the cake. Who brings what?",
			replies: []demoReply{{who: 3, body: "We're coming, all four of us. I'll bring the burek 🥧", likes: 2}, {who: 0, body: "Count us in. @lejla shall I bring the folding table from the garage?", likes: 1},
				{who: 1, replyTo: 3, body: "Yes please, and the extra chairs if they fit in the car."}, {who: 7, body: "I'll do the salads and bring the photo album for after lunch ❤️", likes: 3}}},
		{cat: "Recipes", who: 7, days: 40, read: 0, tags: []string{"baking"}, title: "Nana's walnut roll (orehnjača)",
			body:    "Written down at last, the way Nana makes it.\n\n**Dough:** 500 g flour, 250 ml warm milk, 20 g yeast, 2 eggs, 80 g butter, pinch of salt.\n\n**Filling:** 300 g ground walnuts, 150 g sugar, 100 ml hot milk, lemon zest.\n\n1. Let the dough rise for an hour.\n2. Roll it thin, spread the filling, roll up.\n3. 45 minutes at 180 °C.",
			replies: []demoReply{{who: 3, body: "Made it on Sunday — it disappeared in an hour. Thank you for writing it down!", likes: 3}, {who: 1, body: "She says the secret is to use milk that is *warm, not hot*."}}},
		{cat: "Memories", who: 0, days: 25, read: 0, title: "Summer 1994 at the seaside",
			body:    "Found a box of old prints in the attic. Who remembers the camping trip to Neum? I think that's Uncle Mirza with the huge watermelon.",
			replies: []demoReply{{who: 7, body: "I remember the tent flooding on the second night 😂", likes: 2}, {who: 1, body: "That's definitely Mirza. He carried that watermelon from Mostar."}}},
		{cat: "Questions", kind: "question", who: 3, days: 2, read: -1, title: "Who has the key to the weekend house?",
			body:    "We'd like to go up next weekend. Does anyone have the spare key, or is it still under the flowerpot?",
			replies: []demoReply{{who: 1, body: "@emir has it, I gave it to him at Easter."}}},
	}); err != nil {
		return err
	}

	// 3. A club with events.
	club, cc, err := mkForum("club", "Old Boys FC", "old-boys-fc", "Thursday football for the over-35s. Trainings, matches and the pitch fee.", "⚽", "#15803d", "private", "approval", 0, false)
	if err != nil {
		return err
	}
	addMembers(club, []int{2, 4, 5, 6}, 4)
	if err := seed(club, cc, []demoTopic{
		{cat: "Events", who: 4, days: 4, read: 1, event: now.AddDate(0, 0, 2).Format("2006-01-02") + "T19:30", place: "Pitch 3, Otoka sports centre", title: "Thursday training",
			body:    "Usual time, usual place. Bring both shirts (light and dark). Pitch fee is 10 KM each.",
			replies: []demoReply{{who: 2, body: "In 👍"}, {who: 6, body: "Can't this week, knee is still sore."}, {who: 5, body: "I'm in and I'll bring the new balls.", likes: 2}}},
		{cat: "Events", who: 4, days: 8, read: 0, event: now.AddDate(0, 0, 12).Format("2006-01-02") + "T10:00", place: "Koševo training ground", title: "Friendly against Veterans Grbavica",
			body:    "They invited us for a friendly. 2×35 minutes, then ćevapi. We need at least 12 players — say whether you can make it.",
			replies: []demoReply{{who: 0, body: "Going. I'll ask my brother-in-law if he can play in goal.", likes: 1}, {who: 2, body: "Going!"}}},
		{cat: "Results", who: 2, days: 18, read: 0, tags: []string{"match"}, title: "We won 4–3 (!)",
			body:    "What a game. Down 1–3 at half time, then Dino scored twice. Man of the match: Dino, obviously.",
			replies: []demoReply{{who: 4, body: "My legs still hurt. Worth it 😄", likes: 4}, {who: 5, body: "Great comeback, lads."}}},
		{cat: "Kit & gear", who: 6, days: 26, read: 0, title: "Selling: barely used boots, size 44",
			body: "Adidas Copa, worn twice. 60 KM, or swap for size 43.", closed: true,
			replies: []demoReply{{who: 5, body: "I'll take them if they're still available."}, {who: 6, body: "Sold to Sara. Closing this."}}},
	}); err != nil {
		return err
	}

	// 4. A neighbour's forum, open to everyone on this box, to discover.
	street, sc, err := mkForum("community", "Neighbours of Hamdije Kreševljakovića", "neighbours", "Our street: lost and found, lending things, the house council and the building's boiler.", "🏘️", "#a16207", "box", "open", 1, false)
	if err != nil {
		return err
	}
	addMembers(street, []int{3, 5, 7})
	_ = seed(street, sc, []demoTopic{
		{cat: "General", who: 1, days: 5, read: -1, title: "Who has a ladder I could borrow on Saturday?", body: "Need to clean the gutters. Two metres would do.",
			replies: []demoReply{{who: 3, body: "We have one, ring the bell at no. 12."}}},
		{cat: "Announcements", who: 1, days: 15, read: -1, pinned: 2, title: "House council meeting, Monday 18:00", body: "In the basement room. Topic: replacing the entrance door."},
	})

	// The owner's own bits: a bookmark with a reminder, a draft, an unanswered mention.
	if list, err := s.store.ListTopics(ctx, store.TopicQuery{Viewer: owner, ForumIDs: []int64{help.ID}, Filter: "solved", Limit: 1}); err == nil && len(list) > 0 {
		if p, err := s.store.PostByNumber(ctx, list[0].ID, 2); err == nil {
			at := now.AddDate(0, 0, 2).UTC().Format(time.RFC3339Nano)
			_ = s.store.SaveBookmark(ctx, owner, &store.Bookmark{PostID: p.ID, Note: "Set this up for the family photos", RemindAt: &at})
		}
	}
	if list, err := s.store.ListTopics(ctx, store.TopicQuery{Viewer: owner, ForumIDs: []int64{fam.ID}, Filter: "unsolved", Limit: 1}); err == nil && len(list) > 0 {
		t := list[0]
		if p, err := s.store.PostByNumber(ctx, t.ID, 2); err == nil {
			_, _ = s.store.AddNotification(ctx, &store.Notification{UserID: owner, ForumID: fam.ID, Kind: "mention", TopicID: t.ID, PostID: p.ID, Number: p.Number, ActorID: ids[1], ActorName: name(1), Title: t.Title, Excerpt: excerpt(p.Body, 200)})
		}
	}
	if list, err := s.store.ListTopics(ctx, store.TopicQuery{Viewer: owner, ForumIDs: []int64{help.ID}, Filter: "unsolved", Limit: 3}); err == nil {
		for _, t := range list {
			if t.LastUserID != owner {
				_, _ = s.store.AddNotification(ctx, &store.Notification{UserID: owner, ForumID: help.ID, Kind: "new_topic", TopicID: t.ID, Number: 1, ActorID: t.UserID, ActorName: t.AuthorName, Title: t.Title, Excerpt: excerpt(t.Excerpt, 200)})
			}
		}
	}
	_ = s.store.SaveDraft(ctx, owner, fmt.Sprintf("new:%d", club.ID), []byte(`{"title":"Kit order for the spring","body":"Who wants a new shirt? Sizes and names by Friday."}`))
	_, _ = s.store.DB.ExecContext(ctx, `UPDATE notifications SET pushed = 1, created_at = ? WHERE user_id = ?`, ago(0.1), owner)
	s.log.Info("demo data ready", "forums", 4)
	return nil
}
