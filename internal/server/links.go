package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Forum as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "forum:read", Name: "See your forums", Description: "Read the forums you are in: their categories, topics and posts, what is new and unread for you, your notifications and bookmarks, upcoming events and open questions — for Pocket AI, search and your journal."},
	{Scope: "forum:write", Name: "Post in your forums", Description: "Start a topic and reply as you (Pocket Blog's \"Discuss this post\", Pocket AI when you ask it to), react to a post and mark what you have read; change or remove only the posts that app wrote itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
// Only what is the person's own: the forums they own, whole, with their
// profile, bookmarks and drafts; forums they are only a member of stay put.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "forum.json", Kind: core.KindPocket, Title: "The forums you own with their categories, topics, posts and polls, plus your profile, bookmarks and drafts"},
		{Path: "/api/v1/export.csv", File: "topics.csv", Kind: core.KindCSV, Title: "Every topic in the forums you are in, one per line"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", MaxBytes: 128 << 20, Title: "Brings back forums you owned on another box, as yours; topics already here are left alone"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(home|summary|search|similar|topics|forums|notifications|bookmarks|events|day/\d{4}-\d{2}-\d{2})$`), "forum:read"},
	{"GET", regexp.MustCompile(`^/api/v1/forums/[a-z0-9-]+(/(members|tags|categories))?$`), "forum:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(topics|posts)/\d+$`), "forum:read"},
	{"POST", regexp.MustCompile(`^/api/v1/topics$`), "forum:write"},
	{"POST", regexp.MustCompile(`^/api/v1/topics/\d+/(posts|read)$`), "forum:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/posts/\d+$`), "forum:write"},
	{"POST", regexp.MustCompile(`^/api/v1/posts/\d+/react$`), "forum:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "forum" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

// withLinkApp marks a context as acting for another app (tests, and the auth middleware).
func withLinkApp(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, linkAppKey, slug)
}
