package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Profile is how a person shows up in every forum.
type Profile struct {
	UserID   int64  `json:"user_id"`
	Handle   string `json:"handle"`
	Name     string `json:"name"`
	Bio      string `json:"bio"`
	Color    string `json:"color"`
	AvatarID int64  `json:"avatar_id"`
}

// Forum is a community: its categories, members and rules.
type Forum struct {
	ID           int64    `json:"id"`
	UserID       int64    `json:"owner_id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	About        string   `json:"about"`
	Emoji        string   `json:"emoji"`
	Color        string   `json:"color"`
	Visibility   string   `json:"visibility"`  // private | box | public
	JoinPolicy   string   `json:"join_policy"` // open | approval | invite
	GuestPosts   bool     `json:"guest_posts"`
	ApproveFirst int      `json:"approve_first"`
	Rules        string   `json:"rules"`
	Welcome      string   `json:"welcome"`
	SlowMode     int      `json:"slow_mode"`
	EditMinutes  int      `json:"edit_minutes"`
	BlockedWords []string `json:"blocked_words"`
	HeldWords    []string `json:"held_words"`
	Reactions    []string `json:"reactions"`
	Archived     bool     `json:"archived"`
	Source       string   `json:"source"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

// DefaultReactions are what a forum offers until its owner picks others.
var DefaultReactions = []string{"❤️", "👍", "😂", "🎉", "😮", "🙏"}

// ReactionList is the forum's reactions, the default ones when none are set.
func (f *Forum) ReactionList() []string {
	if len(f.Reactions) == 0 {
		return DefaultReactions
	}
	return f.Reactions
}

// Member is a person's place in a forum.
type Member struct {
	ForumID       int64   `json:"forum_id"`
	UserID        int64   `json:"user_id"`
	Role          string  `json:"role"`   // owner | moderator | member
	Status        string  `json:"status"` // active | pending | banned
	Title         string  `json:"title"`
	Note          string  `json:"note"`
	Level         string  `json:"level"`
	Digest        bool    `json:"digest"`
	BannedUntil   *string `json:"banned_until"`
	BanReason     string  `json:"ban_reason"`
	SilencedUntil *string `json:"silenced_until"`
	InvitedBy     int64   `json:"invited_by"`
	PostsN        int     `json:"posts_n"`
	TopicsN       int     `json:"topics_n"`
	LikesGiven    int     `json:"likes_given"`
	LikesGot      int     `json:"likes_got"`
	SolutionsN    int     `json:"solutions_n"`
	JoinedAt      string  `json:"joined_at"`
	LastSeenAt    *string `json:"last_seen_at"`
	LastPostAt    *string `json:"last_post_at"`

	// From the profile and the account.
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Color  string `json:"color"`
	Avatar int64  `json:"avatar_id"`
	Email  string `json:"-"`
}

// IsMod is true for the owner and the moderators.
func (m *Member) IsMod() bool {
	return m != nil && m.Status == "active" && (m.Role == "owner" || m.Role == "moderator")
}

// Active is true for a member who may read and post.
func (m *Member) Active() bool { return m != nil && m.Status == "active" }

// Invite is a link that lets people join a forum.
type Invite struct {
	ID        int64   `json:"id"`
	ForumID   int64   `json:"forum_id"`
	UserID    int64   `json:"user_id"`
	Token     string  `json:"token"`
	Note      string  `json:"note"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	MaxUses   int     `json:"max_uses"`
	Uses      int     `json:"uses"`
	ExpiresAt *string `json:"expires_at"`
	Accounts  bool    `json:"accounts"`
	Revoked   bool    `json:"revoked"`
	CreatedAt string  `json:"created_at"`
}

// Category groups topics inside a forum.
type Category struct {
	ID        int64  `json:"id"`
	ForumID   int64  `json:"forum_id"`
	ParentID  int64  `json:"parent_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	About     string `json:"about"`
	Emoji     string `json:"emoji"`
	Color     string `json:"color"`
	Position  int    `json:"position"`
	Starts    string `json:"starts"` // all | mods
	QA        bool   `json:"qa"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"created_at"`

	Topics int    `json:"topics"`
	Posts  int    `json:"posts"`
	Latest string `json:"latest_at"`
	Level  string `json:"level"` // the person's own level here
}

// Poster is one of the people in a topic (for the row of faces).
type Poster struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Topic is a conversation: its first post and the replies.
type Topic struct {
	ID           int64    `json:"id"`
	ForumID      int64    `json:"forum_id"`
	CategoryID   int64    `json:"category_id"`
	UserID       int64    `json:"user_id"`
	AuthorName   string   `json:"author_name"`
	Title        string   `json:"title"`
	Kind         string   `json:"kind"`  // discussion | question | announcement | event
	State        string   `json:"state"` // visible | pending | hidden | deleted
	Closed       bool     `json:"closed"`
	Archived     bool     `json:"archived"`
	Pinned       int      `json:"pinned"`
	PinnedUntil  *string  `json:"pinned_until"`
	Unlisted     bool     `json:"unlisted"`
	SolvedPostID int64    `json:"solved_post_id"`
	EventStart   string   `json:"event_start"`
	EventEnd     string   `json:"event_end"`
	EventPlace   string   `json:"event_place"`
	CloseAt      *string  `json:"close_at"`
	SlowMode     int      `json:"slow_mode"`
	MovedTo      int64    `json:"moved_to"`
	Tags         []string `json:"tags"`
	Excerpt      string   `json:"excerpt"`
	Views        int      `json:"views"`
	PostsN       int      `json:"posts_n"`
	Highest      int      `json:"highest"`
	LikesN       int      `json:"likes_n"`
	Posters      []Poster `json:"posters"`
	LastPostAt   *string  `json:"last_post_at"`
	LastUserID   int64    `json:"last_user_id"`
	LastName     string   `json:"last_name"`
	BumpedAt     string   `json:"bumped_at"`
	Source       string   `json:"source"`
	SourceRef    string   `json:"source_ref"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`

	// For the person asking.
	LastRead int    `json:"last_read"`
	Unread   int    `json:"unread"`
	IsNew    bool   `json:"is_new"`
	Level    string `json:"level"`
	Visited  bool   `json:"visited"`
}

// Post is one message in a topic.
type Post struct {
	ID           int64          `json:"id"`
	TopicID      int64          `json:"topic_id"`
	ForumID      int64          `json:"forum_id"`
	UserID       int64          `json:"user_id"`
	AuthorName   string         `json:"author_name"`
	Number       int            `json:"number"`
	ReplyTo      int            `json:"reply_to"`
	Body         string         `json:"body"`
	State        string         `json:"state"`
	HiddenReason string         `json:"hidden_reason"`
	Wiki         bool           `json:"wiki"`
	Edits        int            `json:"edits"`
	EditedAt     *string        `json:"edited_at"`
	LikesN       int            `json:"likes_n"`
	Reactions    map[string]int `json:"reactions"`
	GuestEmail   string         `json:"-"`
	Source       string         `json:"source"`
	SourceRef    string         `json:"source_ref"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`

	// For the page.
	Mine    []string `json:"mine"` // the reactions the person gave
	Handle  string   `json:"handle"`
	Color   string   `json:"color"`
	Avatar  int64    `json:"avatar_id"`
	Role    string   `json:"role"`
	Title   string   `json:"title"`
	Replies int      `json:"replies"`
	Poll    *Poll    `json:"poll,omitempty"`
	Marked  bool     `json:"bookmarked"`
}

// Revision is an earlier version of a post.
type Revision struct {
	ID        int64  `json:"id"`
	PostID    int64  `json:"post_id"`
	UserID    int64  `json:"user_id"`
	Editor    string `json:"editor"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// Poll is a question with choices, inside a post.
type Poll struct {
	ID       int64    `json:"id"`
	PostID   int64    `json:"post_id"`
	TopicID  int64    `json:"topic_id"`
	ForumID  int64    `json:"forum_id"`
	UserID   int64    `json:"user_id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Multi    bool     `json:"multi"`
	Public   bool     `json:"public"`
	ClosesAt *string  `json:"closes_at"`
	Closed   bool     `json:"closed"`

	Counts []int      `json:"counts"`
	Voters int        `json:"voters"`
	Mine   []int      `json:"mine"`
	Who    [][]string `json:"who,omitempty"`
}

// Notification is something for one person to see.
type Notification struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"-"`
	ForumID   int64   `json:"forum_id"`
	Kind      string  `json:"kind"`
	TopicID   int64   `json:"topic_id"`
	PostID    int64   `json:"post_id"`
	Number    int     `json:"number"`
	ActorID   int64   `json:"actor_id"`
	ActorName string  `json:"actor_name"`
	Title     string  `json:"title"`
	Excerpt   string  `json:"excerpt"`
	ReadAt    *string `json:"read_at"`
	Pushed    bool    `json:"-"`
	CreatedAt string  `json:"created_at"`

	ForumSlug  string `json:"forum_slug"`
	ForumName  string `json:"forum_name"`
	ForumEmoji string `json:"forum_emoji"`
}

// Flag is someone telling the moderators about a post.
type Flag struct {
	ID        int64   `json:"id"`
	ForumID   int64   `json:"forum_id"`
	PostID    int64   `json:"post_id"`
	UserID    int64   `json:"user_id"`
	Reason    string  `json:"reason"`
	Note      string  `json:"note"`
	Status    string  `json:"status"`
	HandledBy int64   `json:"handled_by"`
	HandledAt *string `json:"handled_at"`
	CreatedAt string  `json:"created_at"`
	Flagger   string  `json:"flagger"`
}

// LogEntry is one moderation step, for the forum's log.
type LogEntry struct {
	ID         int64  `json:"id"`
	ForumID    int64  `json:"forum_id"`
	UserID     int64  `json:"user_id"`
	ActorName  string `json:"actor_name"`
	Action     string `json:"action"`
	TargetKind string `json:"target_kind"`
	TargetID   int64  `json:"target_id"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}

// Bookmark is a post a person saved, with an optional reminder.
type Bookmark struct {
	ID        int64   `json:"id"`
	PostID    int64   `json:"post_id"`
	Note      string  `json:"note"`
	RemindAt  *string `json:"remind_at"`
	Reminded  bool    `json:"reminded"`
	CreatedAt string  `json:"created_at"`

	TopicID    int64  `json:"topic_id"`
	TopicTitle string `json:"topic_title"`
	Number     int    `json:"number"`
	Excerpt    string `json:"excerpt"`
	AuthorName string `json:"author_name"`
	ForumSlug  string `json:"forum_slug"`
	ForumName  string `json:"forum_name"`
	ForumEmoji string `json:"forum_emoji"`
}

// Upload is a picture or a file in a post, or an avatar.
type Upload struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	ForumID   int64  `json:"forum_id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Bytes     int    `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Data      []byte `json:"-"`
	Thumb     []byte `json:"-"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Reading and writing.
	HomeTab    string `json:"home_tab"`    // latest | unread | new | top
	Density    string `json:"density"`     // comfortable | compact
	AutoWatch  bool   `json:"auto_watch"`  // follow (track) a topic you reply to; watch the ones you start
	Preview    bool   `json:"preview"`     // the composer shows the post as it will look
	LoadImages bool   `json:"load_images"` // show pictures in posts (off: a tap shows them)

	// Other apps.
	NotesFolder string `json:"notes_folder"` // where saved topics go in Pocket Notes
	FilesFolder string `json:"files_folder"` // where exports go in Pocket Files
	ChatTo      string `json:"chat_to"`      // the Pocket Chat conversation topics are shared in
	LearnDeck   string `json:"learn_deck"`   // the Pocket Learn deck answers go to
	ProjectTeam int64  `json:"project_team"` // the Pocket Projects team issues go to
	AIHelp      bool   `json:"ai_help"`

	// Notifications: pushed as they happen (unless the forum is open in a
	// browser), and a summary email.
	NotifyMentions  bool   `json:"notify_mentions"`
	NotifyReplies   bool   `json:"notify_replies"`
	NotifyWatching  bool   `json:"notify_watching"`
	NotifyNewTopics bool   `json:"notify_new_topics"`
	NotifyReview    bool   `json:"notify_review"`
	NotifyLikes     bool   `json:"notify_likes"`
	NotifyHour      int    `json:"notify_hour"`
	NotifyURL       string `json:"notify_url"`
	NotifyFormat    string `json:"notify_format"` // apprise | ntfy | json
	Digest          string `json:"digest"`        // off | daily | weekly (through Pocket Email)
	DigestDay       int    `json:"digest_day"`    // 1 = Monday … 7 = Sunday

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// CleanTags trims, de-duplicates and caps a list of tags.
func CleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
