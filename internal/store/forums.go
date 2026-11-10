package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ---- profiles ----

var reHandle = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,29}$`)

// ValidHandle reports whether a handle can be used for @mentions.
func ValidHandle(h string) bool { return reHandle.MatchString(strings.ToLower(h)) }

// Slugify turns a name into lowercase ASCII words joined by sep
// ("Emir Kovač" → "emir-kovac").
func Slugify(name, sep string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ':
			b.WriteString("dj")
			dash = false
		case r == 'ß':
			b.WriteString("ss")
			dash = false
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteString(sep)
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), sep)
}

// EnsureProfile returns a person's profile, making one (with a free handle
// from their name or email) the first time.
func (s *Store) EnsureProfile(ctx context.Context, uid int64) (*Profile, error) {
	if p, err := s.GetProfile(ctx, uid); err == nil {
		return p, nil
	}
	u, err := s.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	base := Slugify(strings.Fields(u.Name + " x")[0], "")
	if u.Name == "" || len(base) < 2 {
		base = Slugify(strings.Split(u.Email, "@")[0], ".")
	}
	if len(base) < 2 {
		base = "member"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	for i := 0; i < 1000; i++ {
		h := base
		if i > 0 {
			h = fmt.Sprintf("%s%d", base, i+1)
		}
		_, err := s.DB.ExecContext(ctx, `INSERT INTO profiles (user_id, handle) VALUES (?, ?)`, uid, h)
		if err == nil {
			return s.GetProfile(ctx, uid)
		}
		if !strings.Contains(err.Error(), "UNIQUE") {
			return nil, err
		}
		if p, err := s.GetProfile(ctx, uid); err == nil {
			return p, nil
		}
	}
	return nil, ErrConflict
}

const profileCols = `p.user_id, p.handle, u.name, p.bio, p.color, COALESCE(p.avatar_id,0)`

func scanProfile(row interface{ Scan(...any) error }) (*Profile, error) {
	var p Profile
	if err := row.Scan(&p.UserID, &p.Handle, &p.Name, &p.Bio, &p.Color, &p.AvatarID); err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// GetProfile is one person's profile.
func (s *Store) GetProfile(ctx context.Context, uid int64) (*Profile, error) {
	return scanProfile(s.DB.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles p JOIN users u ON u.id = p.user_id WHERE p.user_id = ?`, uid))
}

// ProfileByHandle finds the person an @handle means.
func (s *Store) ProfileByHandle(ctx context.Context, handle string) (*Profile, error) {
	return scanProfile(s.DB.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles p JOIN users u ON u.id = p.user_id WHERE p.handle = ? COLLATE NOCASE`, strings.TrimPrefix(handle, "@")))
}

// UpdateProfile saves a person's handle, line and avatar.
func (s *Store) UpdateProfile(ctx context.Context, p *Profile) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE profiles SET handle = ?, bio = ?, color = ?, avatar_id = ? WHERE user_id = ?`, p.Handle, p.Bio, p.Color, nullI(&p.AvatarID), p.UserID)
	return conflict(err)
}

// ---- forums ----

const forumCols = `id, user_id, slug, name, about, emoji, color, visibility, join_policy, guest_posts, approve_first, rules, welcome, slow_mode, edit_minutes, blocked_words, held_words, reactions, archived, source, created_at, updated_at`

func scanForum(row interface{ Scan(...any) error }) (*Forum, error) {
	var f Forum
	var bw, hw, re string
	if err := row.Scan(&f.ID, &f.UserID, &f.Slug, &f.Name, &f.About, &f.Emoji, &f.Color, &f.Visibility, &f.JoinPolicy, &f.GuestPosts, &f.ApproveFirst, &f.Rules, &f.Welcome,
		&f.SlowMode, &f.EditMinutes, &bw, &hw, &re, &f.Archived, &f.Source, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(bw), &f.BlockedWords)
	_ = json.Unmarshal([]byte(hw), &f.HeldWords)
	_ = json.Unmarshal([]byte(re), &f.Reactions)
	if f.BlockedWords == nil {
		f.BlockedWords = []string{}
	}
	if f.HeldWords == nil {
		f.HeldWords = []string{}
	}
	if f.Reactions == nil {
		f.Reactions = []string{}
	}
	return &f, nil
}

func strList(v []string) string {
	if v == nil {
		v = []string{}
	}
	return toJSON(v)
}

// FreeForumSlug is the slug itself or the first free one after it.
func (s *Store) FreeForumSlug(ctx context.Context, slug string, except int64) string {
	if slug == "" {
		slug = "forum"
	}
	for i := 1; ; i++ {
		c := slug
		if i > 1 {
			c = fmt.Sprintf("%s-%d", slug, i)
		}
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM forums WHERE slug = ? AND id != ?`, c, except).Scan(&n)
		if n == 0 {
			return c
		}
	}
}

// CreateForum adds a forum with its owner as the first member and the
// given categories.
func (s *Store) CreateForum(ctx context.Context, f *Forum, cats []Category) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO forums (user_id, slug, name, about, emoji, color, visibility, join_policy, guest_posts, approve_first, rules, welcome, slow_mode, edit_minutes, blocked_words, held_words, reactions, source)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, f.UserID, f.Slug, f.Name, f.About, f.Emoji, f.Color, f.Visibility, f.JoinPolicy, f.GuestPosts, f.ApproveFirst, f.Rules, f.Welcome,
		f.SlowMode, f.EditMinutes, strList(f.BlockedWords), strList(f.HeldWords), strList(f.Reactions), f.Source)
	if err != nil {
		return conflict(err)
	}
	f.ID, _ = res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO members (forum_id, user_id, role, status, level) VALUES (?, ?, 'owner', 'active', 'tracking')`, f.ID, f.UserID); err != nil {
		return err
	}
	for i, c := range cats {
		if _, err := tx.ExecContext(ctx, `INSERT INTO categories (forum_id, user_id, name, slug, about, emoji, color, position, starts, qa) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			f.ID, f.UserID, c.Name, c.Slug, c.About, c.Emoji, c.Color, i, firstNonEmpty(c.Starts, "all"), c.QA); err != nil {
			return conflict(err)
		}
	}
	return tx.Commit()
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

// GetForum is one forum by id.
func (s *Store) GetForum(ctx context.Context, id int64) (*Forum, error) {
	return scanForum(s.DB.QueryRowContext(ctx, `SELECT `+forumCols+` FROM forums WHERE id = ?`, id))
}

// GetForumBySlug is one forum by its address.
func (s *Store) GetForumBySlug(ctx context.Context, slug string) (*Forum, error) {
	return scanForum(s.DB.QueryRowContext(ctx, `SELECT `+forumCols+` FROM forums WHERE slug = ?`, strings.ToLower(slug)))
}

// ForumBySource is a person's forum that came from elsewhere (an import).
func (s *Store) ForumBySource(ctx context.Context, uid int64, source string) (*Forum, error) {
	return scanForum(s.DB.QueryRowContext(ctx, `SELECT `+forumCols+` FROM forums WHERE user_id = ? AND source = ? ORDER BY id LIMIT 1`, uid, source))
}

// UpdateForum saves a forum's settings.
func (s *Store) UpdateForum(ctx context.Context, f *Forum) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE forums SET slug=?, name=?, about=?, emoji=?, color=?, visibility=?, join_policy=?, guest_posts=?, approve_first=?, rules=?, welcome=?, slow_mode=?, edit_minutes=?,
		blocked_words=?, held_words=?, reactions=?, archived=?, user_id=?, updated_at=? WHERE id=?`,
		f.Slug, f.Name, f.About, f.Emoji, f.Color, f.Visibility, f.JoinPolicy, f.GuestPosts, f.ApproveFirst, f.Rules, f.Welcome, f.SlowMode, f.EditMinutes,
		strList(f.BlockedWords), strList(f.HeldWords), strList(f.Reactions), f.Archived, f.UserID, Now(), f.ID)
	return conflict(err)
}

// DeleteForum removes a forum and everything in it.
func (s *Store) DeleteForum(ctx context.Context, id int64) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM post_search WHERE rowid IN (SELECT id FROM posts WHERE forum_id = ?)`, id); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM forums WHERE id = ?`, id)
	return err
}

func (s *Store) forums(ctx context.Context, q string, args ...any) ([]Forum, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Forum{}
	for rows.Next() {
		f, err := scanForum(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// MyForums are the forums a person belongs to (or asked to join).
func (s *Store) MyForums(ctx context.Context, uid int64) ([]Forum, error) {
	return s.forums(ctx, `SELECT `+forumCols+` FROM forums WHERE id IN (SELECT forum_id FROM members WHERE user_id = ? AND status != 'banned') ORDER BY archived, name COLLATE NOCASE`, uid)
}

// DiscoverForums are the forums anyone with an account here may find, that
// the person is not in yet.
func (s *Store) DiscoverForums(ctx context.Context, uid int64) ([]Forum, error) {
	return s.forums(ctx, `SELECT `+forumCols+` FROM forums WHERE visibility IN ('box','public') AND archived = 0
		AND id NOT IN (SELECT forum_id FROM members WHERE user_id = ?) ORDER BY name COLLATE NOCASE`, uid)
}

// AllForums is every forum on the box (background work only).
func (s *Store) AllForums(ctx context.Context) ([]Forum, error) {
	return s.forums(ctx, `SELECT `+forumCols+` FROM forums ORDER BY id`)
}

// ForumStats are a forum's numbers for its About page.
type ForumStats struct {
	Topics     int `json:"topics"`
	Posts      int `json:"posts"`
	Members    int `json:"members"`
	Pending    int `json:"pending_members"`
	WeekTopics int `json:"week_topics"`
	WeekPosts  int `json:"week_posts"`
	Solved     int `json:"solved"`
}

// Stats counts a forum's topics, posts and members.
func (s *Store) Stats(ctx context.Context, fid int64, since string) ForumStats {
	var st ForumStats
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN solved_post_id IS NOT NULL THEN 1 ELSE 0 END),0) FROM topics WHERE forum_id = ? AND state = 'visible'`, since, fid).Scan(&st.Topics, &st.WeekTopics, &st.Solved)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END),0) FROM posts WHERE forum_id = ? AND state = 'visible'`, since, fid).Scan(&st.Posts, &st.WeekPosts)
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN status='active' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END),0) FROM members WHERE forum_id = ?`, fid).Scan(&st.Members, &st.Pending)
	return st
}

// ---- members ----

const memberCols = `m.forum_id, m.user_id, m.role, m.status, m.title, m.note, m.level, m.digest, m.banned_until, m.ban_reason, m.silenced_until, COALESCE(m.invited_by,0),
	m.posts_n, m.topics_n, m.likes_given, m.likes_got, m.solutions_n, m.joined_at, m.last_seen_at, m.last_post_at,
	u.name, COALESCE(p.handle,''), COALESCE(p.color,''), COALESCE(p.avatar_id,0), u.email`

const memberFrom = ` FROM members m JOIN users u ON u.id = m.user_id LEFT JOIN profiles p ON p.user_id = m.user_id `

func scanMember(row interface{ Scan(...any) error }) (*Member, error) {
	var m Member
	if err := row.Scan(&m.ForumID, &m.UserID, &m.Role, &m.Status, &m.Title, &m.Note, &m.Level, &m.Digest, &m.BannedUntil, &m.BanReason, &m.SilencedUntil, &m.InvitedBy,
		&m.PostsN, &m.TopicsN, &m.LikesGiven, &m.LikesGot, &m.SolutionsN, &m.JoinedAt, &m.LastSeenAt, &m.LastPostAt,
		&m.Name, &m.Handle, &m.Color, &m.Avatar, &m.Email); err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

// GetMember is a person's membership of a forum (ErrNotFound when none).
func (s *Store) GetMember(ctx context.Context, fid, uid int64) (*Member, error) {
	return scanMember(s.DB.QueryRowContext(ctx, `SELECT `+memberCols+memberFrom+`WHERE m.forum_id = ? AND m.user_id = ?`, fid, uid))
}

// AddMember puts a person in a forum (or changes how they are in it).
func (s *Store) AddMember(ctx context.Context, m *Member) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO members (forum_id, user_id, role, status, note, level, invited_by) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(forum_id, user_id) DO UPDATE SET role = excluded.role, status = excluded.status, note = excluded.note`,
		m.ForumID, m.UserID, firstNonEmpty(m.Role, "member"), firstNonEmpty(m.Status, "active"), m.Note, firstNonEmpty(m.Level, "normal"), nullI(&m.InvitedBy))
	return err
}

// UpdateMember saves a member's role, status, title and preferences.
func (s *Store) UpdateMember(ctx context.Context, m *Member) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE members SET role=?, status=?, title=?, level=?, digest=?, banned_until=?, ban_reason=?, silenced_until=? WHERE forum_id=? AND user_id=?`,
		m.Role, m.Status, m.Title, m.Level, m.Digest, m.BannedUntil, m.BanReason, m.SilencedUntil, m.ForumID, m.UserID)
	return err
}

// RemoveMember takes a person out of a forum (their posts stay).
func (s *Store) RemoveMember(ctx context.Context, fid, uid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM members WHERE forum_id = ? AND user_id = ?`, fid, uid)
	return err
}

// TouchMember notes that a member was here.
func (s *Store) TouchMember(ctx context.Context, fid, uid int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE members SET last_seen_at = ? WHERE forum_id = ? AND user_id = ?`, Now(), fid, uid)
}

// ListMembers lists a forum's people, by status ("" = all), filtered by a name.
func (s *Store) ListMembers(ctx context.Context, fid int64, status, q string, limit int) ([]Member, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	where := `WHERE m.forum_id = ?`
	args := []any{fid}
	if status != "" {
		where += ` AND m.status = ?`
		args = append(args, status)
	}
	if q = strings.TrimSpace(strings.TrimPrefix(q, "@")); q != "" {
		where += ` AND (u.name LIKE '%' || ? || '%' OR p.handle LIKE ? || '%')`
		args = append(args, q, q)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+memberCols+memberFrom+where+` ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'moderator' THEN 1 ELSE 2 END, m.posts_n DESC, u.name COLLATE NOCASE LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// MemberIDs are the active members of a forum (mods only: the owner and the moderators).
func (s *Store) MemberIDs(ctx context.Context, fid int64, mods bool) []int64 {
	q := `SELECT user_id FROM members WHERE forum_id = ? AND status = 'active'`
	if mods {
		q += ` AND role IN ('owner','moderator')`
	}
	return s.ids(ctx, q, fid)
}

func (s *Store) ids(ctx context.Context, q string, args ...any) []int64 {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// RecountMember works a member's numbers out again from their posts.
func (s *Store) RecountMember(ctx context.Context, fid, uid int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE members SET
		posts_n = (SELECT COUNT(1) FROM posts WHERE forum_id = ?1 AND user_id = ?2 AND state = 'visible'),
		topics_n = (SELECT COUNT(1) FROM topics WHERE forum_id = ?1 AND user_id = ?2 AND state = 'visible'),
		likes_got = (SELECT COALESCE(SUM(likes_n),0) FROM posts WHERE forum_id = ?1 AND user_id = ?2 AND state = 'visible'),
		likes_given = (SELECT COUNT(1) FROM reactions r JOIN posts p ON p.id = r.post_id WHERE p.forum_id = ?1 AND r.user_id = ?2),
		solutions_n = (SELECT COUNT(1) FROM topics t JOIN posts p ON p.id = t.solved_post_id WHERE t.forum_id = ?1 AND p.user_id = ?2 AND t.state = 'visible'),
		last_post_at = (SELECT MAX(created_at) FROM posts WHERE forum_id = ?1 AND user_id = ?2)
		WHERE forum_id = ?1 AND user_id = ?2`, fid, uid)
}

// SharedForums are the forums two people are both active in.
func (s *Store) SharedForums(ctx context.Context, a, b int64) []int64 {
	return s.ids(ctx, `SELECT x.forum_id FROM members x JOIN members y ON y.forum_id = x.forum_id AND y.user_id = ? AND y.status = 'active'
		WHERE x.user_id = ? AND x.status IN ('active','banned')`, a, b)
}

// ---- invites ----

const inviteCols = `id, forum_id, user_id, token, note, email, role, max_uses, uses, expires_at, accounts, revoked, created_at`

func scanInvite(row interface{ Scan(...any) error }) (*Invite, error) {
	var i Invite
	if err := row.Scan(&i.ID, &i.ForumID, &i.UserID, &i.Token, &i.Note, &i.Email, &i.Role, &i.MaxUses, &i.Uses, &i.ExpiresAt, &i.Accounts, &i.Revoked, &i.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &i, nil
}

// CreateInvite adds an invite link.
func (s *Store) CreateInvite(ctx context.Context, i *Invite) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO invites (forum_id, user_id, token, note, email, role, max_uses, expires_at, accounts) VALUES (?,?,?,?,?,?,?,?,?)`,
		i.ForumID, i.UserID, i.Token, i.Note, i.Email, i.Role, i.MaxUses, i.ExpiresAt, i.Accounts)
	if err != nil {
		return conflict(err)
	}
	i.ID, _ = res.LastInsertId()
	return nil
}

// ListInvites are a forum's invite links, newest first.
func (s *Store) ListInvites(ctx context.Context, fid int64) ([]Invite, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+inviteCols+` FROM invites WHERE forum_id = ? ORDER BY id DESC`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

// GetInvite is one invite by id.
func (s *Store) GetInvite(ctx context.Context, id int64) (*Invite, error) {
	return scanInvite(s.DB.QueryRowContext(ctx, `SELECT `+inviteCols+` FROM invites WHERE id = ?`, id))
}

// InviteByToken finds an invite from its link.
func (s *Store) InviteByToken(ctx context.Context, token string) (*Invite, error) {
	return scanInvite(s.DB.QueryRowContext(ctx, `SELECT `+inviteCols+` FROM invites WHERE token = ?`, token))
}

// UseInvite counts one more person who came in with an invite.
func (s *Store) UseInvite(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE invites SET uses = uses + 1 WHERE id = ?`, id)
}

// RevokeInvite stops an invite link working.
func (s *Store) RevokeInvite(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE invites SET revoked = 1 WHERE id = ?`, id)
	return err
}

// ---- categories ----

const catCols = `c.id, c.forum_id, COALESCE(c.parent_id,0), c.name, c.slug, c.about, c.emoji, c.color, c.position, c.starts, c.qa, c.archived, c.created_at`

func scanCategory(row interface{ Scan(...any) error }, extra ...any) (*Category, error) {
	var c Category
	dst := []any{&c.ID, &c.ForumID, &c.ParentID, &c.Name, &c.Slug, &c.About, &c.Emoji, &c.Color, &c.Position, &c.Starts, &c.QA, &c.Archived, &c.CreatedAt}
	if err := row.Scan(append(dst, extra...)...); err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// ListCategories are a forum's categories with their numbers and the
// person's own level in each.
func (s *Store) ListCategories(ctx context.Context, fid, uid int64) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+catCols+`,
		(SELECT COUNT(1) FROM topics t WHERE t.category_id = c.id AND t.state = 'visible' AND t.unlisted = 0),
		(SELECT COALESCE(SUM(t.posts_n),0) FROM topics t WHERE t.category_id = c.id AND t.state = 'visible'),
		(SELECT COALESCE(MAX(t.bumped_at),'') FROM topics t WHERE t.category_id = c.id AND t.state = 'visible'),
		COALESCE((SELECT level FROM category_levels l WHERE l.category_id = c.id AND l.user_id = ?), '')
		FROM categories c WHERE c.forum_id = ? ORDER BY c.archived, c.position, c.id`, uid, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c *Category
		var topics, posts int
		var latest, level string
		if c, err = scanCategory(rows, &topics, &posts, &latest, &level); err != nil {
			return nil, err
		}
		c.Topics, c.Posts, c.Latest, c.Level = topics, posts, latest, level
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCategory is one category.
func (s *Store) GetCategory(ctx context.Context, id int64) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+catCols+` FROM categories c WHERE c.id = ?`, id))
}

// CategoryBySlug finds a category of a forum by its address.
func (s *Store) CategoryBySlug(ctx context.Context, fid int64, slug string) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+catCols+` FROM categories c WHERE c.forum_id = ? AND c.slug = ?`, fid, slug))
}

// SaveCategory adds (ID 0) or changes a category.
func (s *Store) SaveCategory(ctx context.Context, uid int64, c *Category) error {
	var parent any
	if c.ParentID > 0 {
		parent = c.ParentID
	}
	if c.ID == 0 {
		var pos int
		_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(position),-1)+1 FROM categories WHERE forum_id = ?`, c.ForumID).Scan(&pos)
		res, err := s.DB.ExecContext(ctx, `INSERT INTO categories (forum_id, user_id, parent_id, name, slug, about, emoji, color, position, starts, qa, archived) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.ForumID, uid, parent, c.Name, c.Slug, c.About, c.Emoji, c.Color, pos, c.Starts, c.QA, c.Archived)
		if err != nil {
			return conflict(err)
		}
		c.ID, _ = res.LastInsertId()
		c.Position = pos
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE categories SET parent_id=?, name=?, slug=?, about=?, emoji=?, color=?, starts=?, qa=?, archived=? WHERE id=?`,
		parent, c.Name, c.Slug, c.About, c.Emoji, c.Color, c.Starts, c.QA, c.Archived, c.ID)
	return conflict(err)
}

// DeleteCategory removes a category; its topics move to another one (0 = none).
func (s *Store) DeleteCategory(ctx context.Context, id, moveTo int64) error {
	var to any
	if moveTo > 0 {
		to = moveTo
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE topics SET category_id = ? WHERE category_id = ?`, to, id); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	return err
}

// ReorderCategories puts a forum's categories in this order.
func (s *Store) ReorderCategories(ctx context.Context, fid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE categories SET position = ? WHERE id = ? AND forum_id = ?`, i, id, fid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetCategoryLevel is how closely a person follows a category ("" = like the forum).
func (s *Store) SetCategoryLevel(ctx context.Context, cid, uid int64, level string) error {
	if level == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM category_levels WHERE category_id = ? AND user_id = ?`, cid, uid)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO category_levels (category_id, user_id, level) VALUES (?,?,?) ON CONFLICT(category_id, user_id) DO UPDATE SET level = excluded.level`, cid, uid, level)
	return err
}

// CategoryLevel is a person's level in a category ("" when they never set one).
func (s *Store) CategoryLevel(ctx context.Context, cid, uid int64) string {
	var l string
	_ = s.DB.QueryRowContext(ctx, `SELECT level FROM category_levels WHERE category_id = ? AND user_id = ?`, cid, uid).Scan(&l)
	return l
}

// UsersWatchingCategory are the people who asked for every new topic in a
// category, or — when they never chose for it — in the whole forum.
func (s *Store) UsersWatchingCategory(ctx context.Context, fid, cid int64) []int64 {
	return s.ids(ctx, `SELECT m.user_id FROM members m LEFT JOIN category_levels l ON l.category_id = ? AND l.user_id = m.user_id
		WHERE m.forum_id = ? AND m.status = 'active' AND COALESCE(l.level, m.level) = 'watching'`, cid, fid)
}

// TagCount is a tag and how many topics carry it.
type TagCount struct {
	Tag string `json:"tag"`
	N   int    `json:"n"`
}

// Tags are the tags used in a forum, most used first.
func (s *Store) Tags(ctx context.Context, fid int64) ([]TagCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.tag, COUNT(1) FROM topic_tags g JOIN topics t ON t.id = g.topic_id AND t.state = 'visible' WHERE g.forum_id = ? GROUP BY g.tag COLLATE NOCASE ORDER BY COUNT(1) DESC, g.tag LIMIT 200`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.N); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

var _ = sql.ErrNoRows
