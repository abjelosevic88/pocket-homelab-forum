package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// ---- notifications ----

const notifCols = `n.id, n.user_id, COALESCE(n.forum_id,0), n.kind, COALESCE(n.topic_id,0), COALESCE(n.post_id,0), n.number, COALESCE(n.actor_id,0), n.actor_name, n.title, n.excerpt, n.read_at, n.pushed, n.created_at,
	COALESCE(f.slug,''), COALESCE(f.name,''), COALESCE(f.emoji,'')`

func scanNotification(row interface{ Scan(...any) error }) (*Notification, error) {
	var n Notification
	if err := row.Scan(&n.ID, &n.UserID, &n.ForumID, &n.Kind, &n.TopicID, &n.PostID, &n.Number, &n.ActorID, &n.ActorName, &n.Title, &n.Excerpt, &n.ReadAt, &n.Pushed, &n.CreatedAt,
		&n.ForumSlug, &n.ForumName, &n.ForumEmoji); err != nil {
		return nil, notFound(err)
	}
	return &n, nil
}

// AddNotification stores something for a person to see. A second one of
// the same kind about the same post (two reactions) only refreshes the first.
func (s *Store) AddNotification(ctx context.Context, n *Notification) (bool, error) {
	if n.PostID > 0 && (n.Kind == "reaction" || n.Kind == "flag" || n.Kind == "pending") {
		var id int64
		err := s.DB.QueryRowContext(ctx, `SELECT id FROM notifications WHERE user_id = ? AND kind = ? AND post_id = ? AND read_at IS NULL`, n.UserID, n.Kind, n.PostID).Scan(&id)
		if err == nil {
			_, err = s.DB.ExecContext(ctx, `UPDATE notifications SET actor_id = ?, actor_name = ?, created_at = ? WHERE id = ?`, nullI(&n.ActorID), n.ActorName, Now(), id)
			n.ID = id
			return false, err
		}
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO notifications (user_id, forum_id, kind, topic_id, post_id, number, actor_id, actor_name, title, excerpt) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		n.UserID, nullI(&n.ForumID), n.Kind, nullI(&n.TopicID), nullI(&n.PostID), n.Number, nullI(&n.ActorID), n.ActorName, n.Title, n.Excerpt)
	if err != nil {
		return false, err
	}
	n.ID, _ = res.LastInsertId()
	return true, nil
}

// ListNotifications are a person's notifications, newest first.
func (s *Store) ListNotifications(ctx context.Context, uid int64, unread bool, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	q := `SELECT ` + notifCols + ` FROM notifications n LEFT JOIN forums f ON f.id = n.forum_id WHERE n.user_id = ?`
	if unread {
		q += ` AND n.read_at IS NULL`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY n.id DESC LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// UnpushedNotifications are the ones not sent to a phone yet (background work).
func (s *Store) UnpushedNotifications(ctx context.Context, olderThan string) ([]Notification, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+notifCols+` FROM notifications n LEFT JOIN forums f ON f.id = n.forum_id
		WHERE n.pushed = 0 AND n.read_at IS NULL AND n.created_at <= ? ORDER BY n.id LIMIT 500`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// MarkPushed notes a notification was sent (or should not be).
func (s *Store) MarkPushed(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE notifications SET pushed = 1 WHERE id = ?`, id)
}

// ReadNotifications marks a person's notifications read (none given = all).
func (s *Store) ReadNotifications(ctx context.Context, uid int64, ids []int64) error {
	if len(ids) == 0 {
		_, err := s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, Now(), uid)
		return err
	}
	for _, id := range ids {
		if _, err := s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND id = ? AND read_at IS NULL`, Now(), uid, id); err != nil {
			return err
		}
	}
	return nil
}

// ReadTopicNotifications marks read what a person was told about a topic up to a post.
func (s *Store) ReadTopicNotifications(ctx context.Context, uid, topicID int64, upTo int) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND topic_id = ? AND number <= ? AND read_at IS NULL`, Now(), uid, topicID, upTo)
}

// UnreadNotifications counts a person's unread notifications.
func (s *Store) UnreadNotifications(ctx context.Context, uid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM notifications WHERE user_id = ? AND read_at IS NULL`, uid).Scan(&n)
	return n
}

// ---- bookmarks ----

// SaveBookmark keeps a post for later (or changes its note and reminder).
func (s *Store) SaveBookmark(ctx context.Context, uid int64, b *Bookmark) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO bookmarks (user_id, post_id, note, remind_at) VALUES (?,?,?,?)
		ON CONFLICT(user_id, post_id) DO UPDATE SET note = excluded.note, remind_at = excluded.remind_at, reminded = 0`, uid, b.PostID, b.Note, b.RemindAt)
	return err
}

// DeleteBookmark forgets a saved post.
func (s *Store) DeleteBookmark(ctx context.Context, uid, postID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM bookmarks WHERE user_id = ? AND post_id = ?`, uid, postID)
	return err
}

const bookmarkSel = `SELECT b.id, b.post_id, b.note, b.remind_at, b.reminded, b.created_at, p.topic_id, t.title, p.number, substr(p.body, 1, 300), COALESCE(u.name, p.author_name), f.slug, f.name, f.emoji, b.user_id
	FROM bookmarks b JOIN posts p ON p.id = b.post_id JOIN topics t ON t.id = p.topic_id JOIN forums f ON f.id = p.forum_id LEFT JOIN users u ON u.id = p.user_id `

func (s *Store) bookmarks(ctx context.Context, q string, args ...any) ([]Bookmark, []int64, error) {
	rows, err := s.DB.QueryContext(ctx, bookmarkSel+q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []Bookmark{}
	var users []int64
	for rows.Next() {
		var b Bookmark
		var uid int64
		if err := rows.Scan(&b.ID, &b.PostID, &b.Note, &b.RemindAt, &b.Reminded, &b.CreatedAt, &b.TopicID, &b.TopicTitle, &b.Number, &b.Excerpt, &b.AuthorName, &b.ForumSlug, &b.ForumName, &b.ForumEmoji, &uid); err != nil {
			return nil, nil, err
		}
		out = append(out, b)
		users = append(users, uid)
	}
	return out, users, rows.Err()
}

// ListBookmarks are a person's saved posts in forums they are still in.
func (s *Store) ListBookmarks(ctx context.Context, uid int64) ([]Bookmark, error) {
	out, _, err := s.bookmarks(ctx, `JOIN members m ON m.forum_id = p.forum_id AND m.user_id = b.user_id AND m.status = 'active' WHERE b.user_id = ? ORDER BY COALESCE(b.remind_at, '9999') , b.id DESC`, uid)
	return out, err
}

// DueBookmarks are the reminders that are due (background work).
func (s *Store) DueBookmarks(ctx context.Context, now string) ([]Bookmark, []int64, error) {
	return s.bookmarks(ctx, `WHERE b.remind_at IS NOT NULL AND b.remind_at <= ? AND b.reminded = 0 LIMIT 200`, now)
}

// BookmarkReminded notes a reminder went out.
func (s *Store) BookmarkReminded(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE bookmarks SET reminded = 1 WHERE id = ?`, id)
}

// ---- drafts ----

// GetDraft is a person's unfinished post for a key.
func (s *Store) GetDraft(ctx context.Context, uid int64, key string) (json.RawMessage, string, error) {
	var d, at string
	if err := s.DB.QueryRowContext(ctx, `SELECT data, updated_at FROM drafts WHERE user_id = ? AND key = ?`, uid, key).Scan(&d, &at); err != nil {
		return nil, "", notFound(err)
	}
	return json.RawMessage(d), at, nil
}

// SaveDraft keeps an unfinished post.
func (s *Store) SaveDraft(ctx context.Context, uid int64, key string, data json.RawMessage) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO drafts (user_id, key, data, updated_at) VALUES (?,?,?,?) ON CONFLICT(user_id, key) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, uid, key, string(data), Now())
	return err
}

// DeleteDraft forgets an unfinished post.
func (s *Store) DeleteDraft(ctx context.Context, uid int64, key string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM drafts WHERE user_id = ? AND key = ?`, uid, key)
}

// Draft is an unfinished post, listed.
type Draft struct {
	Key       string          `json:"key"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt string          `json:"updated_at"`
}

// ListDrafts are a person's unfinished posts.
func (s *Store) ListDrafts(ctx context.Context, uid int64) ([]Draft, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, data, updated_at FROM drafts WHERE user_id = ? ORDER BY updated_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Draft{}
	for rows.Next() {
		var d Draft
		var data string
		if err := rows.Scan(&d.Key, &data, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Data = json.RawMessage(data)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---- flags ----

// AddFlag tells a forum's moderators about a post (one open flag per person and post).
func (s *Store) AddFlag(ctx context.Context, f *Flag) error {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM flags WHERE post_id = ? AND user_id = ? AND status = 'open'`, f.PostID, f.UserID).Scan(&id)
	if err == nil {
		f.ID = id
		_, err = s.DB.ExecContext(ctx, `UPDATE flags SET reason = ?, note = ? WHERE id = ?`, f.Reason, f.Note, id)
		return err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO flags (forum_id, post_id, user_id, reason, note) VALUES (?,?,?,?,?)`, f.ForumID, f.PostID, nullI(&f.UserID), f.Reason, f.Note)
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

const flagCols = `g.id, g.forum_id, g.post_id, COALESCE(g.user_id,0), g.reason, g.note, g.status, COALESCE(g.handled_by,0), g.handled_at, g.created_at, COALESCE(u.name,'')`

func scanFlag(row interface{ Scan(...any) error }) (*Flag, error) {
	var f Flag
	if err := row.Scan(&f.ID, &f.ForumID, &f.PostID, &f.UserID, &f.Reason, &f.Note, &f.Status, &f.HandledBy, &f.HandledAt, &f.CreatedAt, &f.Flagger); err != nil {
		return nil, notFound(err)
	}
	return &f, nil
}

// GetFlag is one flag.
func (s *Store) GetFlag(ctx context.Context, id int64) (*Flag, error) {
	return scanFlag(s.DB.QueryRowContext(ctx, `SELECT `+flagCols+` FROM flags g LEFT JOIN users u ON u.id = g.user_id WHERE g.id = ?`, id))
}

// OpenFlags are the flags waiting in these forums.
func (s *Store) OpenFlags(ctx context.Context, forums []int64) ([]Flag, error) {
	if len(forums) == 0 {
		return []Flag{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(forums)), ",")
	args := []any{}
	for _, f := range forums {
		args = append(args, f)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+flagCols+` FROM flags g LEFT JOIN users u ON u.id = g.user_id WHERE g.forum_id IN (`+ph+`) AND g.status = 'open' ORDER BY g.id LIMIT 300`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Flag{}
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// HandleFlags closes every open flag on a post with one decision.
func (s *Store) HandleFlags(ctx context.Context, postID int64, status string, by int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE flags SET status = ?, handled_by = ?, handled_at = ? WHERE post_id = ? AND status = 'open'`, status, by, Now(), postID)
	return err
}

// ---- moderation log ----

// Log writes one moderation step.
func (s *Store) Log(ctx context.Context, e LogEntry) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO mod_log (forum_id, user_id, actor_name, action, target_kind, target_id, detail) VALUES (?,?,?,?,?,?,?)`,
		e.ForumID, nullI(&e.UserID), e.ActorName, e.Action, e.TargetKind, e.TargetID, clip(e.Detail, 500))
}

// ModLog is a forum's moderation history, newest first.
func (s *Store) ModLog(ctx context.Context, fid int64, limit int) ([]LogEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, forum_id, COALESCE(user_id,0), actor_name, action, target_kind, target_id, detail, created_at FROM mod_log WHERE forum_id = ? ORDER BY id DESC LIMIT ?`, fid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.ForumID, &e.UserID, &e.ActorName, &e.Action, &e.TargetKind, &e.TargetID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- polls ----

// SavePoll adds a poll to a post (or replaces its question and choices).
func (s *Store) SavePoll(ctx context.Context, p *Poll) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO polls (post_id, topic_id, forum_id, user_id, question, options, multi, public, closes_at) VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(post_id) DO UPDATE SET question = excluded.question, options = excluded.options, multi = excluded.multi, public = excluded.public, closes_at = excluded.closes_at`,
		p.PostID, p.TopicID, p.ForumID, nullI(&p.UserID), p.Question, strList(p.Options), p.Multi, p.Public, p.ClosesAt)
	if err != nil {
		return err
	}
	if p.ID == 0 {
		p.ID, _ = res.LastInsertId()
	}
	return s.DB.QueryRowContext(ctx, `SELECT id FROM polls WHERE post_id = ?`, p.PostID).Scan(&p.ID)
}

// GetPoll is one poll with its counts and the viewer's own votes.
func (s *Store) GetPoll(ctx context.Context, id, viewer int64) (*Poll, error) {
	return s.poll(ctx, `id = ?`, id, viewer)
}

// PollOfPost is the poll in a post, if any.
func (s *Store) PollOfPost(ctx context.Context, postID, viewer int64) (*Poll, error) {
	return s.poll(ctx, `post_id = ?`, postID, viewer)
}

func (s *Store) poll(ctx context.Context, where string, arg, viewer int64) (*Poll, error) {
	var p Poll
	var opts string
	if err := s.DB.QueryRowContext(ctx, `SELECT id, post_id, topic_id, forum_id, COALESCE(user_id,0), question, options, multi, public, closes_at, closed FROM polls WHERE `+where, arg).
		Scan(&p.ID, &p.PostID, &p.TopicID, &p.ForumID, &p.UserID, &p.Question, &opts, &p.Multi, &p.Public, &p.ClosesAt, &p.Closed); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(opts), &p.Options)
	p.Counts = make([]int, len(p.Options))
	p.Mine = []int{}
	if p.Public {
		p.Who = make([][]string, len(p.Options))
		for i := range p.Who {
			p.Who[i] = []string{}
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT v.user_id, v.choice, u.name FROM votes v JOIN users u ON u.id = v.user_id WHERE v.poll_id = ?`, p.ID)
	if err != nil {
		return nil, err
	}
	voters := map[int64]bool{}
	for rows.Next() {
		var uid int64
		var c int
		var name string
		if rows.Scan(&uid, &c, &name) != nil || c < 0 || c >= len(p.Options) {
			continue
		}
		p.Counts[c]++
		voters[uid] = true
		if uid == viewer {
			p.Mine = append(p.Mine, c)
		}
		if p.Public {
			p.Who[c] = append(p.Who[c], name)
		}
	}
	rows.Close()
	p.Voters = len(voters)
	return &p, nil
}

// Vote replaces a person's votes in a poll.
func (s *Store) Vote(ctx context.Context, pollID, uid int64, choices []int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM votes WHERE poll_id = ? AND user_id = ?`, pollID, uid); err != nil {
		return err
	}
	for _, c := range choices {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO votes (poll_id, user_id, choice) VALUES (?,?,?)`, pollID, uid, c); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClosePoll stops (or reopens) the voting.
func (s *Store) ClosePoll(ctx context.Context, id int64, closed bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE polls SET closed = ? WHERE id = ?`, closed, id)
	return err
}

// DuePolls closes the polls whose time ran out.
func (s *Store) DuePolls(ctx context.Context, now string) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE polls SET closed = 1 WHERE closed = 0 AND closes_at IS NOT NULL AND closes_at <= ?`, now)
}

// ---- event replies ----

// RSVP is who is coming to an event topic.
type RSVP struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// SetRSVP saves a person's answer to an event ("" takes it back).
func (s *Store) SetRSVP(ctx context.Context, topicID, uid int64, status string) error {
	if status == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM rsvps WHERE topic_id = ? AND user_id = ?`, topicID, uid)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO rsvps (topic_id, user_id, status, updated_at) VALUES (?,?,?,?) ON CONFLICT(topic_id, user_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`, topicID, uid, status, Now())
	return err
}

// RSVPs are the answers to an event.
func (s *Store) RSVPs(ctx context.Context, topicID int64) ([]RSVP, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.user_id, u.name, r.status FROM rsvps r JOIN users u ON u.id = r.user_id WHERE r.topic_id = ? ORDER BY r.updated_at`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RSVP{}
	for rows.Next() {
		var r RSVP
		if err := rows.Scan(&r.UserID, &r.Name, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- uploads ----

// SaveUpload stores a picture or a file.
func (s *Store) SaveUpload(ctx context.Context, u *Upload) error {
	var forum any
	if u.ForumID > 0 {
		forum = u.ForumID
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO uploads (user_id, forum_id, name, mime, bytes, width, height, data, thumb) VALUES (?,?,?,?,?,?,?,?,?)`,
		u.UserID, forum, u.Name, u.Mime, len(u.Data), u.Width, u.Height, u.Data, u.Thumb)
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	u.Bytes = len(u.Data)
	return s.DB.QueryRowContext(ctx, `SELECT created_at FROM uploads WHERE id = ?`, u.ID).Scan(&u.CreatedAt)
}

// GetUpload is one upload with its bytes.
func (s *Store) GetUpload(ctx context.Context, id int64) (*Upload, error) {
	var u Upload
	var forum sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, forum_id, name, mime, bytes, width, height, data, thumb, created_at FROM uploads WHERE id = ?`, id).
		Scan(&u.ID, &u.UserID, &forum, &u.Name, &u.Mime, &u.Bytes, &u.Width, &u.Height, &u.Data, &u.Thumb, &u.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	u.ForumID = forum.Int64
	return &u, nil
}

// MarkUploadsUsed notes the uploads a saved post points at, so the clean-up keeps them.
func (s *Store) MarkUploadsUsed(ctx context.Context, ids []int64) {
	for _, id := range ids {
		_, _ = s.DB.ExecContext(ctx, `UPDATE uploads SET used = 1 WHERE id = ?`, id)
	}
}

// PurgeUploads removes uploads no post or profile ever used, older than a day.
func (s *Store) PurgeUploads(ctx context.Context, before string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM uploads WHERE used = 0 AND created_at < ? AND id NOT IN (SELECT avatar_id FROM profiles WHERE avatar_id IS NOT NULL)`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
