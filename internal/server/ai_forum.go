package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// Pocket AI in the forum: it reads for the person who asks and writes only
// for them — a summary, a draft, a suggestion. It never posts by itself.

func (s *Server) aiReady(w http.ResponseWriter, r *http.Request) (*ai.Client, func(), bool) {
	ctx := r.Context()
	st := s.settingsOf(ctx, actorID(ctx))
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off in Settings → AI")
		return nil, nil, false
	}
	cl, _, why := s.aiFor(ctx, actorID(ctx), st)
	if cl == nil {
		writeErr(w, http.StatusBadRequest, firstNonEmpty(why, "no AI is set up: connect Pocket AI through Pocket Core, or add an OpenAI key in Settings → AI"))
		return nil, nil, false
	}
	select {
	case s.aiSem <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, false
	}
	return cl, func() { <-s.aiSem }, true
}

// topicText is a topic as plain text for the AI, newest posts kept when long.
func (s *Server) topicText(ctx context.Context, t *store.Topic, limit int) string {
	posts, _ := s.store.ListPosts(ctx, t.ID, 0, false, 0, 1000)
	var parts []string
	for _, p := range posts {
		if p.State != "visible" {
			continue
		}
		mark := ""
		if p.ID == t.SolvedPostID {
			mark = " [accepted answer]"
		}
		reply := ""
		if p.ReplyTo > 0 {
			reply = fmt.Sprintf(" (answering #%d)", p.ReplyTo)
		}
		parts = append(parts, fmt.Sprintf("#%d %s%s%s, %d reactions:\n%s", p.Number, p.AuthorName, reply, mark, p.LikesN, clip(mdPlain(p.Body), 3000)))
	}
	text := strings.Join(parts, "\n\n")
	for len([]rune(text)) > limit && len(parts) > 2 {
		parts = append(parts[:1], parts[2:]...)
		text = parts[0] + "\n\n[…earlier replies left out…]\n\n" + strings.Join(parts[1:], "\n\n")
	}
	return clip(text, limit)
}

func (s *Server) handleAISummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		TopicID int64 `json:"topic_id"`
		Since   int   `json:"since"` // only what came after this post number
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	_, t, err := s.topicAccess(ctx, userFrom(ctx), in.TopicID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cl, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	sys := `You summarise a forum topic for someone who has not read it. Write in the language most posts are in. Plain, friendly words.
Format (Markdown): one short paragraph with the gist; then "**Main points**" with 2–6 bullets naming who said what (by name); then, when there is one, "**Answer**" (the accepted answer or what most agreed on) and "**Still open**" (questions nobody answered). Refer to posts as #number. No preamble.`
	prompt := "Topic: " + t.Title + "\n\n" + s.topicText(ctx, t, 24000)
	if in.Since > 0 {
		sys = `You tell someone what they missed in a forum topic since they last read it. Write in the language most posts are in. Plain, friendly words; 2–5 bullets naming who said what, referring to posts as #number. No preamble.`
		prompt = fmt.Sprintf("Topic: %s\nThey read up to #%d. Everything:\n\n%s", t.Title, in.Since, s.topicText(ctx, t, 24000))
	}
	out, err := cl.Complete(ctx, sys, prompt, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": strings.TrimSpace(out)})
}

func (s *Server) handleAIReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		TopicID int64  `json:"topic_id"`
		ReplyTo int    `json:"reply_to"`
		Hint    string `json:"hint"`
		Draft   string `json:"draft"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	_, t, err := s.topicAccess(ctx, u, in.TopicID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cl, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	sys := `You help a forum member write a reply, as them, in the first person. Match the topic's language and tone; be warm, concrete and short (usually 2–6 sentences). Use Markdown sparingly. Never invent facts about the person; when you would need to know something, leave a short [placeholder]. Answer with the reply text only.`
	prompt := fmt.Sprintf("I am %s.\nTopic: %s\n\n%s", s.actorName(u), t.Title, s.topicText(ctx, t, 16000))
	if in.ReplyTo > 0 {
		prompt += fmt.Sprintf("\n\nI am answering post #%d.", in.ReplyTo)
	}
	if h := strings.TrimSpace(in.Hint); h != "" {
		prompt += "\n\nWhat I want to say: " + clip(h, 1000)
	}
	if d := strings.TrimSpace(in.Draft); d != "" {
		prompt += "\n\nMy draft so far (improve and complete it):\n" + clip(d, 6000)
	}
	out, err := cl.Complete(ctx, sys, prompt, false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(out)})
}

// handleAIImprove rewrites what the person is writing: clearer, shorter, friendlier, or just fixed.
func (s *Server) handleAIImprove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Text string `json:"text"`
		How  string `json:"how"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "write something first")
		return
	}
	how := map[string]string{"fix": "Only fix spelling, grammar and punctuation; change nothing else.", "clearer": "Make it clearer and better organised, keeping everything it says.",
		"shorter": "Make it about half as long, keeping what matters.", "friendlier": "Make it warmer and friendlier, keeping what it says."}[in.How]
	if how == "" {
		writeErr(w, http.StatusBadRequest, "fix, clearer, shorter or friendlier")
		return
	}
	cl, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	out, err := cl.Complete(ctx, "You edit a forum post. "+how+" Keep its language, Markdown, @mentions, links and [quote=…]…[/quote] blocks exactly. Answer with the post only.", clip(in.Text, maxBody), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(out)})
}

// handleAISuggest proposes a category, tags and a clearer title for a new topic.
func (s *Server) handleAISuggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Forum string `json:"forum"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	a, err := s.forumAccess(ctx, userFrom(ctx), in.Forum)
	if err != nil || !a.canRead() {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cats, _ := s.store.ListCategories(ctx, a.f.ID, a.uid())
	tags, _ := s.store.Tags(ctx, a.f.ID)
	var cl []string
	for _, c := range cats {
		if c.Archived || (c.Starts == "mods" && !a.mod()) {
			continue
		}
		cl = append(cl, fmt.Sprintf("%d: %s — %s", c.ID, c.Name, c.About))
	}
	var tl []string
	for i, t := range tags {
		if i < 60 {
			tl = append(tl, t.Tag)
		}
	}
	c, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	var out struct {
		Title      string   `json:"title"`
		CategoryID int64    `json:"category_id"`
		Tags       []string `json:"tags"`
		Kind       string   `json:"kind"`
		Why        string   `json:"why"`
	}
	sys := `You help someone post a new topic in a forum. Answer JSON: {"title": a clear, specific title in the post's language (keep theirs when it is fine), "category_id": the best category's number, "tags": up to 3 short lowercase tags (reuse existing ones), "kind": "question" when they ask for help or an answer else "discussion", "why": one short sentence}.`
	prompt := fmt.Sprintf("Categories:\n%s\n\nTags in use: %s\n\nTitle: %s\n\nPost:\n%s", strings.Join(cl, "\n"), strings.Join(tl, ", "), in.Title, clip(in.Body, 6000))
	if err := c.JSON(ctx, sys, prompt, &out); err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	valid := false
	for _, x := range cats {
		if x.ID == out.CategoryID {
			valid = true
		}
	}
	if !valid {
		out.CategoryID = 0
	}
	if !oneOf(out.Kind, []string{"question", "discussion"}) {
		out.Kind = "discussion"
	}
	out.Tags = store.CleanTags(out.Tags)
	if len(out.Tags) > 3 {
		out.Tags = out.Tags[:3]
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAIAsk answers a question from what the forums say, with the topics it used.
func (s *Server) handleAIAsk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Forum    string `json:"forum"`
		Question string `json:"question"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Question) == "" {
		writeErr(w, http.StatusBadRequest, "ask a question")
		return
	}
	var forums []int64
	if in.Forum != "" {
		a, err := s.forumAccess(ctx, u, in.Forum)
		if err != nil || !a.canRead() {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		forums = []int64{a.f.ID}
	} else {
		forums, _, _ = s.readableForums(ctx, u)
	}
	hits, _ := s.store.Search(ctx, store.SearchQuery{Text: stoplessQ(in.Question), ForumIDs: forums, Limit: 30})
	if len(hits) == 0 {
		hits, _ = s.store.Similar(ctx, forums, in.Question, 8)
	}
	seen := map[int64]bool{}
	var ctxParts []string
	sources := []map[string]any{}
	for _, h := range hits {
		if seen[h.TopicID] || len(sources) >= 5 {
			continue
		}
		seen[h.TopicID] = true
		t, err := s.store.GetTopic(ctx, h.TopicID)
		if err != nil {
			continue
		}
		ctxParts = append(ctxParts, fmt.Sprintf("[%d] %s\n%s", len(sources)+1, t.Title, s.topicText(ctx, t, 5000)))
		sources = append(sources, map[string]any{"n": len(sources) + 1, "topic_id": t.ID, "title": t.Title, "solved": t.SolvedPostID > 0, "url": s.topicURL(t, 0)})
	}
	if len(sources) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"answer": "Nothing in your forums talks about that yet. You could ask it as a new topic.", "sources": sources})
		return
	}
	cl, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	sys := `You answer a question using only the forum topics given, numbered [1]…[5]. Answer in the question's language, short and practical. Put the topic number in brackets after each fact, like [2]. When the topics do not answer it, say so plainly. No preamble.`
	out, err := cl.Complete(ctx, sys, "Question: "+clip(in.Question, 1000)+"\n\nTopics:\n\n"+strings.Join(ctxParts, "\n\n"), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": strings.TrimSpace(out), "sources": sources})
}

func stoplessQ(q string) string {
	var keep []string
	for _, w := range strings.Fields(q) {
		t := strings.ToLower(strings.Trim(w, "?!.,:;\"'()"))
		if len([]rune(t)) > 3 {
			keep = append(keep, t)
		}
	}
	if len(keep) > 4 {
		keep = keep[:4]
	}
	return strings.Join(keep, " ")
}

// handleAICheck gives a moderator a second opinion on a flagged or held post.
func (s *Server) handleAICheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		PostID int64 `json:"post_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p, err := s.store.GetPost(ctx, in.PostID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	a, t, err := s.topicAccess(ctx, userFrom(ctx), p.TopicID)
	if err != nil || !a.mod() {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	cl, done, ok := s.aiReady(w, r)
	if !ok {
		return
	}
	defer done()
	var out struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	sys := `You help a forum moderator. Read the post against the forum's rules. Answer JSON {"verdict": "fine" | "look" | "remove", "reason": one or two plain sentences in English}. "look" = borderline, a person should decide. Spam, abuse, hate, threats, doxxing → "remove". Disagreement or bluntness alone is "fine".`
	prompt := fmt.Sprintf("Forum rules:\n%s\n\nTopic: %s\nPost by %s:\n%s", clip(a.f.Rules, 3000), t.Title, p.AuthorName, clip(p.Body, 6000))
	if err := cl.JSON(ctx, sys, prompt, &out); err != nil {
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	if !oneOf(out.Verdict, []string{"fine", "look", "remove"}) {
		out.Verdict = "look"
	}
	writeJSON(w, http.StatusOK, out)
}
