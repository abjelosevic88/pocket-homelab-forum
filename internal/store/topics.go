package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ---- topics ----

const topicCols = `t.id, t.forum_id, COALESCE(t.category_id,0), COALESCE(t.user_id,0), t.author_name, t.title, t.kind, t.state, t.closed, t.archived, t.pinned, t.pinned_until,
	t.unlisted, COALESCE(t.solved_post_id,0), t.event_start, t.event_end, t.event_place, t.close_at, t.slow_mode, COALESCE(t.moved_to,0), t.tags, t.excerpt, t.views, t.posts_n, t.highest,
	t.likes_n, t.posters, t.last_post_at, COALESCE(t.last_user_id,0), t.last_name, t.bumped_at, t.source, t.source_ref, t.created_at, t.updated_at`

func scanTopic(row interface{ Scan(...any) error }, extra ...any) (*Topic, error) {
	var t Topic
	var tags, posters string
	dst := []any{&t.ID, &t.ForumID, &t.CategoryID, &t.UserID, &t.AuthorName, &t.Title, &t.Kind, &t.State, &t.Closed, &t.Archived, &t.Pinned, &t.PinnedUntil,
		&t.Unlisted, &t.SolvedPostID, &t.EventStart, &t.EventEnd, &t.EventPlace, &t.CloseAt, &t.SlowMode, &t.MovedTo, &tags, &t.Excerpt, &t.Views, &t.PostsN, &t.Highest,
		&t.LikesN, &posters, &t.LastPostAt, &t.LastUserID, &t.LastName, &t.BumpedAt, &t.Source, &t.SourceRef, &t.CreatedAt, &t.UpdatedAt}
	if err := row.Scan(append(dst, extra...)...); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(tags), &t.Tags)
	_ = json.Unmarshal([]byte(posters), &t.Posters)
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.Posters == nil {
		t.Posters = []Poster{}
	}
	return &t, nil
}

// GetTopic is one topic, whatever its state.
func (s *Store) GetTopic(ctx context.Context, id int64) (*Topic, error) {
	return scanTopic(s.DB.QueryRowContext(ctx, `SELECT `+topicCols+` FROM topics t WHERE t.id = ?`, id))
}

// TopicBySource finds the topic another app started for one of its things.
func (s *Store) TopicBySource(ctx context.Context, fid int64, source, ref string) (*Topic, error) {
	return scanTopic(s.DB.QueryRowContext(ctx, `SELECT `+topicCols+` FROM topics t WHERE t.forum_id = ? AND t.source = ? AND t.source_ref = ? AND t.state != 'deleted' ORDER BY t.id LIMIT 1`, fid, source, ref))
}

// CreateTopic adds a topic and its first post.
func (s *Store) CreateTopic(ctx context.Context, t *Topic, first *Post) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t.Tags = CleanTags(t.Tags)
	res, err := tx.ExecContext(ctx, `INSERT INTO topics (forum_id, category_id, user_id, author_name, title, kind, state, closed, pinned, unlisted, event_start, event_end, event_place, close_at, tags, excerpt, highest, source, source_ref, created_at, updated_at, bumped_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?)`,
		t.ForumID, nullI(&t.CategoryID), nullI(&t.UserID), t.AuthorName, t.Title, t.Kind, t.State, t.Closed, t.Pinned, t.Unlisted, t.EventStart, t.EventEnd, t.EventPlace, t.CloseAt,
		toJSON(t.Tags), clip(first.Body, 400), t.Source, t.SourceRef, firstNonEmpty(t.CreatedAt, Now()), firstNonEmpty(t.CreatedAt, Now()), firstNonEmpty(t.CreatedAt, Now()))
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	if err := setTags(ctx, tx, t); err != nil {
		return err
	}
	first.TopicID, first.ForumID, first.Number = t.ID, t.ForumID, 1
	if first.CreatedAt == "" {
		first.CreatedAt = t.CreatedAt
	}
	if err := insertPost(ctx, tx, first, t.Title); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.RecountTopic(ctx, t.ID)
	if t.UserID > 0 {
		s.RecountMember(ctx, t.ForumID, t.UserID)
	}
	nt, err := s.GetTopic(ctx, t.ID)
	if err == nil {
		*t = *nt
	}
	return err
}

func setTags(ctx context.Context, tx *sql.Tx, t *Topic) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM topic_tags WHERE topic_id = ?`, t.ID); err != nil {
		return err
	}
	for _, g := range t.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO topic_tags (topic_id, forum_id, tag) VALUES (?,?,?)`, t.ID, t.ForumID, g); err != nil {
			return err
		}
	}
	return nil
}

// UpdateTopic saves a topic's title, category, kind, tags, event and status.
func (s *Store) UpdateTopic(ctx context.Context, t *Topic) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t.Tags = CleanTags(t.Tags)
	var solved any
	if t.SolvedPostID > 0 {
		solved = t.SolvedPostID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE topics SET category_id=?, title=?, kind=?, state=?, closed=?, archived=?, pinned=?, pinned_until=?, unlisted=?, solved_post_id=?,
		event_start=?, event_end=?, event_place=?, close_at=?, slow_mode=?, moved_to=?, tags=?, forum_id=?, updated_at=? WHERE id=?`,
		nullI(&t.CategoryID), t.Title, t.Kind, t.State, t.Closed, t.Archived, t.Pinned, t.PinnedUntil, t.Unlisted, solved,
		t.EventStart, t.EventEnd, t.EventPlace, t.CloseAt, t.SlowMode, nullI(&t.MovedTo), toJSON(t.Tags), t.ForumID, Now(), t.ID); err != nil {
		return err
	}
	if err := setTags(ctx, tx, t); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE post_search SET title = ? WHERE rowid = (SELECT id FROM posts WHERE topic_id = ? AND number = 1)`, t.Title, t.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// RecountTopic works out a topic's numbers, last post and faces again.
func (s *Store) RecountTopic(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE topics SET
		posts_n = (SELECT COUNT(1) FROM posts WHERE topic_id = ?1 AND state = 'visible'),
		likes_n = (SELECT COALESCE(SUM(likes_n),0) FROM posts WHERE topic_id = ?1 AND state = 'visible'),
		highest = MAX(highest, (SELECT COALESCE(MAX(number),0) FROM posts WHERE topic_id = ?1)),
		last_post_at = (SELECT MAX(created_at) FROM posts WHERE topic_id = ?1 AND state = 'visible'),
		last_user_id = (SELECT user_id FROM posts WHERE topic_id = ?1 AND state = 'visible' ORDER BY number DESC LIMIT 1),
		last_name = COALESCE((SELECT COALESCE(u.name, p.author_name) FROM posts p LEFT JOIN users u ON u.id = p.user_id WHERE p.topic_id = ?1 AND p.state = 'visible' ORDER BY p.number DESC LIMIT 1), ''),
		excerpt = COALESCE((SELECT substr(body, 1, 400) FROM posts WHERE topic_id = ?1 AND number = (SELECT MIN(number) FROM posts WHERE topic_id = ?1)), excerpt),
		bumped_at = COALESCE((SELECT MAX(created_at) FROM posts WHERE topic_id = ?1 AND state = 'visible'), bumped_at)
		WHERE id = ?1`, id)
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(p.user_id,0), COALESCE(u.name, p.author_name), COALESCE(pr.color,''), MIN(p.number), MAX(p.number) FROM posts p
		LEFT JOIN users u ON u.id = p.user_id LEFT JOIN profiles pr ON pr.user_id = p.user_id
		WHERE p.topic_id = ? AND p.state = 'visible' GROUP BY COALESCE(p.user_id, 0), CASE WHEN p.user_id IS NULL THEN p.author_name ELSE '' END`, id)
	if err != nil {
		return
	}
	type pp struct {
		Poster
		first, last int
	}
	var all []pp
	for rows.Next() {
		var x pp
		if rows.Scan(&x.ID, &x.Name, &x.Color, &x.first, &x.last) == nil {
			all = append(all, x)
		}
	}
	rows.Close()
	sort.Slice(all, func(i, j int) bool {
		if all[i].first == 1 || all[j].first == 1 {
			return all[i].first == 1
		}
		return all[i].last > all[j].last
	})
	out := []Poster{}
	for i := 0; i < len(all) && i < 5; i++ {
		out = append(out, all[i].Poster)
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE topics SET posters = ? WHERE id = ?`, toJSON(out), id)
}

// AddView counts one more reading of a topic.
func (s *Store) AddView(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE topics SET views = views + 1 WHERE id = ?`, id)
}

// TopicQuery picks topics for a list.
type TopicQuery struct {
	Viewer     int64
	ForumIDs   []int64
	CategoryID int64 // the category and its subcategories
	Tag        string
	Filter     string // latest | unread | new | top | unsolved | noreplies | mine | posted | events | solved
	Kind       string
	AuthorID   int64
	Since      string // top: from this time
	NewSince   string // new: created after this time
	Limit      int
	Offset     int
	Pins       bool // pinned topics first
}

// ListTopics lists the visible topics a query asks for, with the viewer's read state.
func (s *Store) ListTopics(ctx context.Context, q TopicQuery) ([]Topic, error) {
	if len(q.ForumIDs) == 0 {
		return []Topic{}, nil
	}
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 30
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(q.ForumIDs)), ",")
	args := []any{q.Viewer, q.Viewer, q.Viewer}
	where := []string{"t.forum_id IN (" + ph + ")", "t.state = 'visible'", "t.unlisted = 0"}
	for _, id := range q.ForumIDs {
		args = append(args, id)
	}
	if q.CategoryID > 0 {
		where = append(where, "(t.category_id = ? OR t.category_id IN (SELECT id FROM categories WHERE parent_id = ?))")
		args = append(args, q.CategoryID, q.CategoryID)
	}
	if q.Tag != "" {
		where = append(where, "t.id IN (SELECT topic_id FROM topic_tags WHERE tag = ? COLLATE NOCASE)")
		args = append(args, q.Tag)
	}
	if q.Kind != "" {
		where = append(where, "t.kind = ?")
		args = append(args, q.Kind)
	}
	if q.AuthorID > 0 {
		where = append(where, "t.user_id = ?")
		args = append(args, q.AuthorID)
	}
	muted := "COALESCE(r.level,'') != 'muted' AND COALESCE(cl.level,'') != 'muted'"
	order := "t.bumped_at DESC"
	if q.Pins {
		if q.CategoryID > 0 {
			order = "CASE WHEN t.pinned > 0 THEN 1 ELSE 0 END DESC, " + order
		} else {
			order = "CASE WHEN t.pinned = 2 THEN 1 ELSE 0 END DESC, " + order
		}
	}
	unreadExpr := "EXISTS (SELECT 1 FROM posts p WHERE p.topic_id = t.id AND p.state = 'visible' AND p.number > r.last_read)"
	switch q.Filter {
	case "unread":
		where = append(where, "r.topic_id IS NOT NULL", muted, unreadExpr)
		order = "t.bumped_at DESC"
	case "new":
		where = append(where, "r.topic_id IS NULL", muted, "m.user_id IS NOT NULL", "t.created_at > m.joined_at", "t.created_at > ?", "COALESCE(t.user_id,0) != ?")
		args = append(args, firstNonEmpty(q.NewSince, time.Now().AddDate(0, 0, -14).UTC().Format(time.RFC3339Nano)), q.Viewer)
		order = "t.created_at DESC"
	case "top":
		if q.Since != "" {
			where = append(where, "t.created_at >= ?")
			args = append(args, q.Since)
		}
		order = "(t.likes_n * 3 + t.posts_n * 2 + t.views / 5) DESC, t.bumped_at DESC"
	case "unsolved":
		where = append(where, "t.kind = 'question'", "t.solved_post_id IS NULL", "t.closed = 0")
	case "solved":
		where = append(where, "t.solved_post_id IS NOT NULL")
	case "noreplies":
		where = append(where, "t.posts_n <= 1", "t.closed = 0", "t.kind != 'announcement'")
		order = "t.created_at DESC"
	case "mine":
		where = append(where, "t.user_id = ?")
		args = append(args, q.Viewer)
	case "posted":
		where = append(where, "t.id IN (SELECT topic_id FROM posts WHERE user_id = ? AND state = 'visible')")
		args = append(args, q.Viewer)
	case "events":
		where = append(where, "t.kind = 'event'", "t.event_start >= ?")
		args = append(args, time.Now().Format("2006-01-02"))
		order = "t.event_start ASC"
	case "past_events":
		where = append(where, "t.kind = 'event'", "t.event_start != ''", "t.event_start < ?")
		args = append(args, time.Now().Format("2006-01-02"))
		order = "t.event_start DESC"
	default:
		if q.CategoryID == 0 && q.Tag == "" && q.AuthorID == 0 && q.Viewer > 0 {
			where = append(where, muted)
		}
	}
	query := `SELECT ` + topicCols + `, COALESCE(r.last_read,-1), COALESCE(r.level,''), COALESCE(m.joined_at,''),
		(SELECT COUNT(1) FROM posts p WHERE p.topic_id = t.id AND p.state = 'visible' AND p.number > COALESCE(r.last_read, 0))
		FROM topics t LEFT JOIN reads r ON r.topic_id = t.id AND r.user_id = ?
		LEFT JOIN members m ON m.forum_id = t.forum_id AND m.user_id = ? AND m.status = 'active'
		LEFT JOIN category_levels cl ON cl.category_id = t.category_id AND cl.user_id = ?
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, q.Limit, q.Offset)
	return s.topicRows(ctx, q.Viewer, query, args...)
}

func (s *Store) topicRows(ctx context.Context, viewer int64, query string, args ...any) ([]Topic, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cut := time.Now().AddDate(0, 0, -14).UTC().Format(time.RFC3339Nano)
	out := []Topic{}
	for rows.Next() {
		var lastRead, unread int
		var level, joined string
		t, err := scanTopic(rows, &lastRead, &level, &joined, &unread)
		if err != nil {
			return nil, err
		}
		t.Level = level
		if lastRead >= 0 {
			t.Visited, t.LastRead, t.Unread = true, lastRead, unread
		} else if viewer > 0 && joined != "" && t.CreatedAt > joined && t.CreatedAt > cut && t.UserID != viewer {
			t.IsNew = true
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TopicFor is one topic with the viewer's read state.
func (s *Store) TopicFor(ctx context.Context, viewer, id int64) (*Topic, error) {
	list, err := s.topicRows(ctx, viewer, `SELECT `+topicCols+`, COALESCE(r.last_read,-1), COALESCE(r.level,''), COALESCE(m.joined_at,''),
		(SELECT COUNT(1) FROM posts p WHERE p.topic_id = t.id AND p.state = 'visible' AND p.number > COALESCE(r.last_read, 0))
		FROM topics t LEFT JOIN reads r ON r.topic_id = t.id AND r.user_id = ?
		LEFT JOIN members m ON m.forum_id = t.forum_id AND m.user_id = ? AND m.status = 'active'
		WHERE t.id = ?`, viewer, viewer, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Counts per forum of what a person has not read yet.
type UnreadCount struct {
	Unread int `json:"unread"`
	New    int `json:"new"`
}

// UnreadCounts are, per forum, the topics with new replies and the new topics.
func (s *Store) UnreadCounts(ctx context.Context, uid int64, newSince string) map[int64]UnreadCount {
	out := map[int64]UnreadCount{}
	rows, err := s.DB.QueryContext(ctx, `SELECT t.forum_id, COUNT(1) FROM topics t JOIN reads r ON r.topic_id = t.id AND r.user_id = ?1
		JOIN members m ON m.forum_id = t.forum_id AND m.user_id = ?1 AND m.status = 'active'
		LEFT JOIN category_levels cl ON cl.category_id = t.category_id AND cl.user_id = ?1
		WHERE t.state = 'visible' AND t.unlisted = 0 AND r.level != 'muted' AND COALESCE(cl.level,'') != 'muted'
		AND EXISTS (SELECT 1 FROM posts p WHERE p.topic_id = t.id AND p.state = 'visible' AND p.number > r.last_read) GROUP BY t.forum_id`, uid)
	if err == nil {
		for rows.Next() {
			var f int64
			var n int
			if rows.Scan(&f, &n) == nil {
				c := out[f]
				c.Unread = n
				out[f] = c
			}
		}
		rows.Close()
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT t.forum_id, COUNT(1) FROM topics t JOIN members m ON m.forum_id = t.forum_id AND m.user_id = ?1 AND m.status = 'active'
		LEFT JOIN reads r ON r.topic_id = t.id AND r.user_id = ?1
		LEFT JOIN category_levels cl ON cl.category_id = t.category_id AND cl.user_id = ?1
		WHERE r.topic_id IS NULL AND t.state = 'visible' AND t.unlisted = 0 AND t.created_at > m.joined_at AND t.created_at > ?2
		AND COALESCE(t.user_id,0) != ?1 AND COALESCE(cl.level,'') != 'muted' GROUP BY t.forum_id`, uid, newSince)
	if err == nil {
		for rows.Next() {
			var f int64
			var n int
			if rows.Scan(&f, &n) == nil {
				c := out[f]
				c.New = n
				out[f] = c
			}
		}
		rows.Close()
	}
	return out
}

// ---- posts ----

const postCols = `p.id, p.topic_id, p.forum_id, COALESCE(p.user_id,0), COALESCE(u.name, p.author_name), p.number, p.reply_to, p.body, p.state, p.hidden_reason, p.wiki, p.edits, p.edited_at,
	p.likes_n, p.reactions, p.guest_email, p.source, p.source_ref, p.created_at, p.updated_at,
	COALESCE(pr.handle,''), COALESCE(pr.color,''), COALESCE(pr.avatar_id,0), COALESCE(m.role,''), COALESCE(m.title,'')`

const postFrom = ` FROM posts p LEFT JOIN users u ON u.id = p.user_id LEFT JOIN profiles pr ON pr.user_id = p.user_id LEFT JOIN members m ON m.forum_id = p.forum_id AND m.user_id = p.user_id `

func scanPost(row interface{ Scan(...any) error }, extra ...any) (*Post, error) {
	var p Post
	var reactions string
	dst := []any{&p.ID, &p.TopicID, &p.ForumID, &p.UserID, &p.AuthorName, &p.Number, &p.ReplyTo, &p.Body, &p.State, &p.HiddenReason, &p.Wiki, &p.Edits, &p.EditedAt,
		&p.LikesN, &reactions, &p.GuestEmail, &p.Source, &p.SourceRef, &p.CreatedAt, &p.UpdatedAt, &p.Handle, &p.Color, &p.Avatar, &p.Role, &p.Title}
	if err := row.Scan(append(dst, extra...)...); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(reactions), &p.Reactions)
	if p.Reactions == nil {
		p.Reactions = map[string]int{}
	}
	p.Mine = []string{}
	return &p, nil
}

func insertPost(ctx context.Context, tx *sql.Tx, p *Post, title string) error {
	at := firstNonEmpty(p.CreatedAt, Now())
	res, err := tx.ExecContext(ctx, `INSERT INTO posts (topic_id, forum_id, user_id, author_name, number, reply_to, body, state, wiki, guest_email, source, source_ref, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.TopicID, p.ForumID, nullI(&p.UserID), p.AuthorName, p.Number, p.ReplyTo, p.Body, firstNonEmpty(p.State, "visible"), p.Wiki,
		p.GuestEmail, p.Source, p.SourceRef, at, at)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	p.CreatedAt, p.UpdatedAt = at, at
	if p.State == "" {
		p.State = "visible"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO post_search (rowid, title, body) VALUES (?, ?, ?)`, p.ID, title, p.Body)
	return err
}

// CreatePost adds a reply at the end of a topic.
func (s *Store) CreatePost(ctx context.Context, p *Post) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT highest FROM topics WHERE id = ?`, p.TopicID).Scan(&n); err != nil {
		return notFound(err)
	}
	p.Number = n + 1
	if _, err := tx.ExecContext(ctx, `UPDATE topics SET highest = ? WHERE id = ?`, p.Number, p.TopicID); err != nil {
		return err
	}
	if err := insertPost(ctx, tx, p, ""); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.RecountTopic(ctx, p.TopicID)
	if p.UserID > 0 {
		s.RecountMember(ctx, p.ForumID, p.UserID)
	}
	return nil
}

// GetPost is one post with its author's face.
func (s *Store) GetPost(ctx context.Context, id int64) (*Post, error) {
	return scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+postFrom+`WHERE p.id = ?`, id))
}

// PostByNumber is a topic's post with that number.
func (s *Store) PostByNumber(ctx context.Context, topicID int64, number int) (*Post, error) {
	return scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+postFrom+`WHERE p.topic_id = ? AND p.number = ?`, topicID, number))
}

// ListPosts are a topic's posts in order. states picks which ones
// (visible always; pending/hidden/deleted for moderators and their authors).
func (s *Store) ListPosts(ctx context.Context, topicID, viewer int64, mod bool, from, limit int) ([]Post, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+postCols+`, (SELECT COUNT(1) FROM posts q WHERE q.topic_id = p.topic_id AND q.reply_to = p.number AND q.state = 'visible'),
		EXISTS (SELECT 1 FROM bookmarks b WHERE b.post_id = p.id AND b.user_id = ?)`+postFrom+`
		WHERE p.topic_id = ? AND p.number >= ? AND (p.state IN ('visible','deleted') OR ? OR (p.user_id = ? AND p.state IN ('pending','hidden')))
		ORDER BY p.number LIMIT ?`, viewer, topicID, from, mod, viewer, limit)
	if err != nil {
		return nil, err
	}
	out := []Post{}
	for rows.Next() {
		var replies int
		var marked bool
		p, err := scanPost(rows, &replies, &marked)
		if err != nil {
			rows.Close()
			return nil, err
		}
		p.Replies, p.Marked = replies, marked
		out = append(out, *p)
	}
	rows.Close()
	if viewer > 0 && len(out) > 0 {
		mine := map[int64][]string{}
		rows, err := s.DB.QueryContext(ctx, `SELECT r.post_id, r.emoji FROM reactions r JOIN posts p ON p.id = r.post_id WHERE p.topic_id = ? AND r.user_id = ?`, topicID, viewer)
		if err == nil {
			for rows.Next() {
				var id int64
				var e string
				if rows.Scan(&id, &e) == nil {
					mine[id] = append(mine[id], e)
				}
			}
			rows.Close()
		}
		for i := range out {
			if m := mine[out[i].ID]; m != nil {
				out[i].Mine = m
			}
		}
	}
	return out, nil
}

// EditPost changes a post's text, keeping what it said before.
func (s *Store) EditPost(ctx context.Context, p *Post, oldBody, oldTitle string, editorID int64, editor, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO revisions (post_id, user_id, editor, title, body, reason) VALUES (?,?,?,?,?,?)`, p.ID, nullI(&editorID), editor, oldTitle, oldBody, reason); err != nil {
		return err
	}
	now := Now()
	if _, err := tx.ExecContext(ctx, `UPDATE posts SET body = ?, edits = edits + 1, edited_at = ?, updated_at = ? WHERE id = ?`, p.Body, now, now, p.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE post_search SET body = ? WHERE rowid = ?`, p.Body, p.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.Edits++
	p.EditedAt = &now
	if p.Number == 1 {
		s.RecountTopic(ctx, p.TopicID)
	}
	return nil
}

// Revisions are a post's earlier versions, newest first.
func (s *Store) Revisions(ctx context.Context, postID int64) ([]Revision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, post_id, COALESCE(user_id,0), editor, title, body, reason, created_at FROM revisions WHERE post_id = ? ORDER BY id DESC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.PostID, &r.UserID, &r.Editor, &r.Title, &r.Body, &r.Reason, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetPostState shows, holds, hides or deletes a post and counts again.
func (s *Store) SetPostState(ctx context.Context, p *Post, state, reason string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE posts SET state = ?, hidden_reason = ?, updated_at = ? WHERE id = ?`, state, reason, Now(), p.ID); err != nil {
		return err
	}
	p.State, p.HiddenReason = state, reason
	s.RecountTopic(ctx, p.TopicID)
	if p.UserID > 0 {
		s.RecountMember(ctx, p.ForumID, p.UserID)
	}
	return nil
}

// SetWiki lets every member edit a post (or stops it).
func (s *Store) SetWiki(ctx context.Context, id int64, on bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE posts SET wiki = ? WHERE id = ?`, on, id)
	return err
}

// RecentPosts are a person's latest visible posts in the given forums.
func (s *Store) RecentPosts(ctx context.Context, uid int64, forums []int64, limit int) ([]Post, []string, error) {
	if len(forums) == 0 {
		return []Post{}, nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(forums)), ",")
	args := []any{uid}
	for _, f := range forums {
		args = append(args, f)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+postCols+`, t.title`+postFrom+` JOIN topics t ON t.id = p.topic_id
		WHERE p.user_id = ? AND p.forum_id IN (`+ph+`) AND p.state = 'visible' AND t.state = 'visible' AND t.unlisted = 0 ORDER BY p.id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out, titles := []Post{}, []string{}
	for rows.Next() {
		var title string
		p, err := scanPost(rows, &title)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *p)
		titles = append(titles, title)
	}
	return out, titles, rows.Err()
}

// PostsOn are a person's posts made on one local day, with their topic titles.
func (s *Store) PostsOn(ctx context.Context, uid int64, from, to string) ([]Post, []string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+postCols+`, t.title`+postFrom+` JOIN topics t ON t.id = p.topic_id
		WHERE p.user_id = ? AND p.created_at >= ? AND p.created_at < ? AND p.state = 'visible' AND t.state = 'visible' ORDER BY p.id`, uid, from, to)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out, titles := []Post{}, []string{}
	for rows.Next() {
		var title string
		p, err := scanPost(rows, &title)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *p)
		titles = append(titles, title)
	}
	return out, titles, rows.Err()
}

// PendingPosts are the posts (and topics) waiting for a moderator.
func (s *Store) PendingPosts(ctx context.Context, forums []int64) ([]Post, []string, error) {
	if len(forums) == 0 {
		return []Post{}, nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(forums)), ",")
	args := []any{}
	for _, f := range forums {
		args = append(args, f)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+postCols+`, t.title`+postFrom+` JOIN topics t ON t.id = p.topic_id
		WHERE p.forum_id IN (`+ph+`) AND p.state = 'pending' AND t.state != 'deleted' ORDER BY p.id LIMIT 200`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out, titles := []Post{}, []string{}
	for rows.Next() {
		var title string
		p, err := scanPost(rows, &title)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, *p)
		titles = append(titles, title)
	}
	return out, titles, rows.Err()
}

// CountPostsBy counts a person's visible posts in a forum (for "hold the first posts").
func (s *Store) CountPostsBy(ctx context.Context, fid, uid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM posts WHERE forum_id = ? AND user_id = ? AND state = 'visible'`, fid, uid).Scan(&n)
	return n
}

// LastPostAt is when a person last posted in a forum ("" never).
func (s *Store) LastPostAt(ctx context.Context, fid, uid int64, topicID int64) string {
	var at sql.NullString
	if topicID > 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT MAX(created_at) FROM posts WHERE topic_id = ? AND user_id = ?`, topicID, uid).Scan(&at)
	} else {
		_ = s.DB.QueryRowContext(ctx, `SELECT MAX(created_at) FROM posts WHERE forum_id = ? AND user_id = ?`, fid, uid).Scan(&at)
	}
	return at.String
}

// ---- reactions ----

// ToggleReaction gives or takes back a reaction; true when it was given.
func (s *Store) ToggleReaction(ctx context.Context, p *Post, uid int64, emoji string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM reactions WHERE post_id = ? AND user_id = ? AND emoji = ?`, p.ID, uid, emoji)
	if err != nil {
		return false, err
	}
	added := false
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO reactions (post_id, user_id, emoji) VALUES (?,?,?)`, p.ID, uid, emoji); err != nil {
			return false, err
		}
		added = true
	}
	s.recountReactions(ctx, p)
	s.RecountTopic(ctx, p.TopicID)
	if p.UserID > 0 {
		s.RecountMember(ctx, p.ForumID, p.UserID)
	}
	s.RecountMember(ctx, p.ForumID, uid)
	return added, nil
}

func (s *Store) recountReactions(ctx context.Context, p *Post) {
	rows, err := s.DB.QueryContext(ctx, `SELECT emoji, COUNT(1) FROM reactions WHERE post_id = ? GROUP BY emoji`, p.ID)
	if err != nil {
		return
	}
	m := map[string]int{}
	total := 0
	for rows.Next() {
		var e string
		var n int
		if rows.Scan(&e, &n) == nil {
			m[e] = n
			total += n
		}
	}
	rows.Close()
	p.Reactions, p.LikesN = m, total
	_, _ = s.DB.ExecContext(ctx, `UPDATE posts SET reactions = ?, likes_n = ? WHERE id = ?`, toJSON(m), total, p.ID)
}

// Reactor is who gave a reaction.
type Reactor struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Emoji  string `json:"emoji"`
}

// Reactors list who reacted to a post, and with what.
func (s *Store) Reactors(ctx context.Context, postID int64) ([]Reactor, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.user_id, u.name, r.emoji FROM reactions r JOIN users u ON u.id = r.user_id WHERE r.post_id = ? ORDER BY r.created_at`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reactor{}
	for rows.Next() {
		var r Reactor
		if err := rows.Scan(&r.UserID, &r.Name, &r.Emoji); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- reading ----

// MarkRead moves a person's place in a topic forward to number.
func (s *Store) MarkRead(ctx context.Context, topicID, uid int64, number int) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO reads (topic_id, user_id, last_read, visited_at) VALUES (?,?,?,?)
		ON CONFLICT(topic_id, user_id) DO UPDATE SET last_read = MAX(last_read, excluded.last_read), visited_at = excluded.visited_at`, topicID, uid, number, Now())
}

// VisitedAt is when a person last opened a topic ("" never).
func (s *Store) VisitedAt(ctx context.Context, topicID, uid int64) string {
	var at sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT visited_at FROM reads WHERE topic_id = ? AND user_id = ?`, topicID, uid).Scan(&at)
	return at.String
}

// SetTopicLevel is how closely a person follows a topic.
func (s *Store) SetTopicLevel(ctx context.Context, topicID, uid int64, level string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO reads (topic_id, user_id, level) VALUES (?,?,?) ON CONFLICT(topic_id, user_id) DO UPDATE SET level = excluded.level`, topicID, uid, level)
	return err
}

// RaiseTopicLevel sets a level only when the person has none yet (or a lower one).
func (s *Store) RaiseTopicLevel(ctx context.Context, topicID, uid int64, level string) {
	rank := map[string]int{"": 0, "normal": 1, "tracking": 2, "watching": 3, "muted": 4}
	var cur string
	_ = s.DB.QueryRowContext(ctx, `SELECT level FROM reads WHERE topic_id = ? AND user_id = ?`, topicID, uid).Scan(&cur)
	if rank[level] > rank[cur] && cur != "muted" {
		_ = s.SetTopicLevel(ctx, topicID, uid, level)
	}
}

// TopicLevel is a person's own level for a topic ("" = not chosen).
func (s *Store) TopicLevel(ctx context.Context, topicID, uid int64) string {
	var l string
	_ = s.DB.QueryRowContext(ctx, `SELECT level FROM reads WHERE topic_id = ? AND user_id = ?`, topicID, uid).Scan(&l)
	return l
}

// Watchers are the people watching a topic (for each new post).
func (s *Store) Watchers(ctx context.Context, topicID int64) []int64 {
	return s.ids(ctx, `SELECT user_id FROM reads WHERE topic_id = ? AND level = 'watching'`, topicID)
}

// ---- moving posts ----

// MovePosts puts posts (by id, in their order) at the end of another
// topic, renumbering them; a reply to a post that did not come along
// loses its "in reply to".
func (s *Store) MovePosts(ctx context.Context, ids []int64, into int64) error {
	if len(ids) == 0 {
		return nil
	}
	type mp struct {
		id       int64
		from     int64
		number   int
		replyTo  int
		forum    int64
		userID   int64
		created  string
		newNumer int
	}
	var list []mp
	for _, id := range ids {
		var x mp
		if err := s.DB.QueryRowContext(ctx, `SELECT id, topic_id, number, reply_to, forum_id, COALESCE(user_id,0), created_at FROM posts WHERE id = ?`, id).Scan(&x.id, &x.from, &x.number, &x.replyTo, &x.forum, &x.userID, &x.created); err != nil {
			return notFound(err)
		}
		list = append(list, x)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].created != list[j].created {
			return list[i].created < list[j].created
		}
		return list[i].number < list[j].number
	})
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var highest int
	var forum int64
	if err := tx.QueryRowContext(ctx, `SELECT highest, forum_id FROM topics WHERE id = ?`, into).Scan(&highest, &forum); err != nil {
		return notFound(err)
	}
	renum := map[string]int{}
	for i := range list {
		highest++
		list[i].newNumer = highest
		renum[fmt.Sprintf("%d:%d", list[i].from, list[i].number)] = highest
	}
	var title string
	_ = tx.QueryRowContext(ctx, `SELECT title FROM topics WHERE id = ?`, into).Scan(&title)
	for _, x := range list {
		reply := renum[fmt.Sprintf("%d:%d", x.from, x.replyTo)]
		if _, err := tx.ExecContext(ctx, `UPDATE posts SET topic_id = ?, forum_id = ?, number = ?, reply_to = ? WHERE id = ?`, into, forum, x.newNumer, reply, x.id); err != nil {
			return err
		}
		t := ""
		if x.newNumer == 1 {
			t = title
		}
		if _, err := tx.ExecContext(ctx, `UPDATE post_search SET title = ? WHERE rowid = ?`, t, x.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE topics SET highest = ? WHERE id = ?`, highest, into); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	seen := map[int64]bool{into: true}
	s.RecountTopic(ctx, into)
	for _, x := range list {
		if !seen[x.from] {
			seen[x.from] = true
			s.RecountTopic(ctx, x.from)
		}
		if x.userID > 0 {
			s.RecountMember(ctx, x.forum, x.userID)
			s.RecountMember(ctx, forum, x.userID)
		}
	}
	return nil
}

// ---- search ----

// SearchQuery is a search over the posts a person can read.
type SearchQuery struct {
	Text       string
	ForumIDs   []int64
	CategoryID int64
	Tag        string
	AuthorID   int64
	Kind       string
	Solved     string // "" | yes | no
	TitleOnly  bool
	Limit      int
}

// Hit is one search result.
type Hit struct {
	PostID     int64    `json:"post_id"`
	TopicID    int64    `json:"topic_id"`
	ForumID    int64    `json:"forum_id"`
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Snippet    string   `json:"snippet"`
	AuthorName string   `json:"author_name"`
	CreatedAt  string   `json:"created_at"`
	Kind       string   `json:"kind"`
	Solved     bool     `json:"solved"`
	CategoryID int64    `json:"category_id"`
	Tags       []string `json:"tags"`
	PostsN     int      `json:"posts_n"`
}

// FTSQuery turns what a person typed into a safe full-text query where every
// word must appear (the last one as a prefix); "" when nothing usable is left.
func FTSQuery(text string, any bool) string {
	var words []string
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 2 || unicode.IsDigit([]rune(w)[0]) {
			words = append(words, `"`+strings.ReplaceAll(w, `"`, ``)+`"`)
		}
		if len(words) >= 12 {
			break
		}
	}
	if len(words) == 0 {
		return ""
	}
	words[len(words)-1] += "*"
	if any {
		return strings.Join(words, " OR ")
	}
	return strings.Join(words, " ")
}

// Search finds posts by their words.
func (s *Store) Search(ctx context.Context, q SearchQuery) ([]Hit, error) {
	match := FTSQuery(q.Text, false)
	if match == "" || len(q.ForumIDs) == 0 {
		return []Hit{}, nil
	}
	if q.TitleOnly {
		match = "title : (" + match + ")"
	}
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 40
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(q.ForumIDs)), ",")
	args := []any{match}
	for _, f := range q.ForumIDs {
		args = append(args, f)
	}
	where := ""
	if q.CategoryID > 0 {
		where += " AND (t.category_id = ? OR t.category_id IN (SELECT id FROM categories WHERE parent_id = ?))"
		args = append(args, q.CategoryID, q.CategoryID)
	}
	if q.Tag != "" {
		where += " AND t.id IN (SELECT topic_id FROM topic_tags WHERE tag = ? COLLATE NOCASE)"
		args = append(args, q.Tag)
	}
	if q.AuthorID > 0 {
		where += " AND p.user_id = ?"
		args = append(args, q.AuthorID)
	}
	if q.Kind != "" {
		where += " AND t.kind = ?"
		args = append(args, q.Kind)
	}
	switch q.Solved {
	case "yes":
		where += " AND t.solved_post_id IS NOT NULL"
	case "no":
		where += " AND t.kind = 'question' AND t.solved_post_id IS NULL"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id, p.topic_id, p.forum_id, p.number, t.title, snippet(post_search, 1, '⟦', '⟧', '…', 18), COALESCE(u.name, p.author_name), p.created_at,
		t.kind, t.solved_post_id IS NOT NULL, COALESCE(t.category_id,0), t.tags, t.posts_n
		FROM post_search JOIN posts p ON p.id = post_search.rowid JOIN topics t ON t.id = p.topic_id LEFT JOIN users u ON u.id = p.user_id
		WHERE post_search MATCH ? AND p.forum_id IN (`+ph+`) AND p.state = 'visible' AND t.state = 'visible' AND t.unlisted = 0`+where+`
		ORDER BY bm25(post_search, 4.0, 1.0) + CASE WHEN p.number = 1 THEN -1.5 ELSE 0 END LIMIT ?`, append(args, q.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hit{}
	for rows.Next() {
		var h Hit
		var tags string
		if err := rows.Scan(&h.PostID, &h.TopicID, &h.ForumID, &h.Number, &h.Title, &h.Snippet, &h.AuthorName, &h.CreatedAt, &h.Kind, &h.Solved, &h.CategoryID, &tags, &h.PostsN); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tags), &h.Tags)
		if h.Tags == nil {
			h.Tags = []string{}
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Similar are the topics whose titles share words with a title being written.
func (s *Store) Similar(ctx context.Context, forums []int64, title string, limit int) ([]Hit, error) {
	match := FTSQuery(stopless(title), true)
	if match == "" || len(forums) == 0 {
		return []Hit{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(forums)), ",")
	args := []any{"title : (" + match + ")"}
	for _, f := range forums {
		args = append(args, f)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id, p.topic_id, p.forum_id, t.title, t.excerpt, t.kind, t.solved_post_id IS NOT NULL, t.posts_n, t.created_at FROM post_search
		JOIN posts p ON p.id = post_search.rowid JOIN topics t ON t.id = p.topic_id
		WHERE post_search MATCH ? AND p.number = 1 AND p.forum_id IN (`+ph+`) AND t.state = 'visible' AND t.unlisted = 0 ORDER BY bm25(post_search) LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.PostID, &h.TopicID, &h.ForumID, &h.Title, &h.Snippet, &h.Kind, &h.Solved, &h.PostsN, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Number = 1
		h.Snippet = clip(h.Snippet, 160)
		h.Tags = []string{}
		out = append(out, h)
	}
	return out, rows.Err()
}

var stopWords = map[string]bool{"the": true, "and": true, "for": true, "with": true, "how": true, "what": true, "why": true, "can": true, "does": true, "is": true, "are": true,
	"to": true, "of": true, "in": true, "on": true, "a": true, "an": true, "my": true, "do": true, "it": true, "be": true, "or": true, "at": true, "i": true, "we": true, "you": true,
	"kako": true, "sta": true, "što": true, "šta": true, "je": true, "da": true, "li": true, "na": true, "za": true, "se": true, "su": true, "ili": true, "od": true}

func stopless(t string) string {
	var keep []string
	for _, w := range strings.Fields(strings.ToLower(t)) {
		if !stopWords[strings.Trim(w, "?!.,:;\"'()")] {
			keep = append(keep, w)
		}
	}
	return strings.Join(keep, " ")
}

// clip shortens s to n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
