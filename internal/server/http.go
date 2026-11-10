package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("forum"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Get("/feeds/{forum}", s.handleFeed)

	r.Route("/api/v1", func(r chi.Router) {
		// public forums and invitations: no account needed
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Second))
			r.Get("/public/forums/{forum}", s.handlePublicForum)
			r.Get("/public/forums/{forum}/topics", s.handlePublicTopics)
			r.Post("/public/forums/{forum}/topics", s.handleGuestTopic)
			r.Get("/public/topics/{id}", s.handlePublicTopic)
			r.Post("/public/topics/{id}/posts", s.handleGuestPost)
			r.Get("/public/uploads/{id}", s.handlePublicUpload)
			r.Get("/public/invites/{token}", s.handlePublicInvite)
			r.Post("/public/invites/{token}/signup", s.handleInviteSignup)
		})

		// live updates: no timeout
		r.With(s.requireAuth).Get("/stream", s.handleStream)

		// uploads, imports and AI can take longer than a request may
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/uploads", s.handleUpload)
			r.Post("/import", s.handleImport)
			r.Post("/forums/{forum}/import/discourse", s.handleImportDiscourse)
			r.Get("/export.json", s.handleExportJSON)
			r.Post("/ai/summary", s.handleAISummary)
			r.Post("/ai/reply", s.handleAIReply)
			r.Post("/ai/improve", s.handleAIImprove)
			r.Post("/ai/suggest", s.handleAISuggest)
			r.Post("/ai/ask", s.handleAIAsk)
			r.Post("/ai/check", s.handleAICheck)
			r.Post("/apps/translate", s.handleTranslate)
			r.Post("/apps/photos/{uuid}/copy", s.handlePhotoCopy)
			r.Post("/digest/test", s.handleDigestTest)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				// for the person, and for the rest of the box: Pocket Day, Search, AI
				r.Get("/home", s.handleHome)
				r.Get("/summary", s.handleSummary)
				r.Get("/day/{date}", s.handleDay)
				r.Get("/search", s.handleSearch)
				r.Get("/similar", s.handleSimilar)
				r.Get("/events", s.handleEvents)
				r.Get("/notifications", s.handleNotifications)
				r.Post("/notifications/read", s.handleReadNotifications)
				r.Get("/bookmarks", s.handleBookmarks)
				r.Get("/drafts", s.handleDrafts)
				r.Get("/drafts/{key}", s.handleDraft)
				r.Put("/drafts/{key}", s.handleDraft)
				r.Delete("/drafts/{key}", s.handleDraft)
				r.Put("/profile", s.handleUpdateProfile)
				r.Get("/people/{id}", s.handlePerson)
				r.Get("/review", s.handleReview)
				r.Get("/uploads/{id}", s.handleGetUpload)
				r.Post("/invites/{token}/accept", s.handleAcceptInvite)
				r.Delete("/invites/{id}", s.handleRevokeInvite)

				// forums
				r.Get("/forums", s.handleForums)
				r.Post("/forums", s.handleCreateForum)
				r.Get("/forums/{forum}", s.handleGetForum)
				r.Put("/forums/{forum}", s.handleUpdateForum)
				r.Delete("/forums/{forum}", s.handleDeleteForum)
				r.Post("/forums/{forum}/join", s.handleJoin)
				r.Post("/forums/{forum}/leave", s.handleLeave)
				r.Put("/forums/{forum}/me", s.handleMyMembership)
				r.Get("/forums/{forum}/members", s.handleMembers)
				r.Post("/forums/{forum}/members", s.handleAddMember)
				r.Put("/forums/{forum}/members/{uid}", s.handleUpdateMember)
				r.Delete("/forums/{forum}/members/{uid}", s.handleRemoveMember)
				r.Get("/forums/{forum}/people", s.handlePeople)
				r.Get("/forums/{forum}/invites", s.handleListInvites)
				r.Post("/forums/{forum}/invites", s.handleCreateInvite)
				r.Get("/forums/{forum}/categories", s.handleListCategories)
				r.Post("/forums/{forum}/categories", s.handleCreateCategory)
				r.Put("/forums/{forum}/categories/order", s.handleReorderCategories)
				r.Get("/forums/{forum}/tags", s.handleTags)
				r.Get("/forums/{forum}/log", s.handleModLog)
				r.Put("/categories/{id}", s.handleUpdateCategory)
				r.Delete("/categories/{id}", s.handleDeleteCategory)
				r.Put("/categories/{id}/level", s.handleCategoryLevel)

				// topics and posts
				r.Get("/topics", s.handleTopics)
				r.Post("/topics", s.handleCreateTopic)
				r.Get("/topics/{id}", s.handleGetTopic)
				r.Put("/topics/{id}", s.handleUpdateTopic)
				r.Delete("/topics/{id}", s.handleDeleteTopic)
				r.Post("/topics/{id}/status", s.handleTopicStatus)
				r.Post("/topics/{id}/merge", s.handleMergeTopic)
				r.Post("/topics/{id}/split", s.handleSplitTopic)
				r.Post("/topics/{id}/read", s.handleReadTopic)
				r.Put("/topics/{id}/level", s.handleTopicLevel)
				r.Post("/topics/{id}/solve", s.handleSolve)
				r.Post("/topics/{id}/rsvp", s.handleRSVP)
				r.Post("/topics/{id}/posts", s.handleCreatePost)
				r.Get("/posts/{id}", s.handleGetPost)
				r.Put("/posts/{id}", s.handleUpdatePost)
				r.Delete("/posts/{id}", s.handleDeletePost)
				r.Post("/posts/{id}/restore", s.handleRestorePost)
				r.Get("/posts/{id}/revisions", s.handleRevisions)
				r.Post("/posts/{id}/react", s.handleReact)
				r.Get("/posts/{id}/reactions", s.handleReactors)
				r.Post("/posts/{id}/flag", s.handleFlag)
				r.Post("/posts/{id}/bookmark", s.handleBookmark)
				r.Delete("/posts/{id}/bookmark", s.handleBookmark)
				r.Post("/posts/{id}/wiki", s.handleWiki)
				r.Post("/posts/{id}/hide", s.handleHidePost)
				r.Post("/posts/{id}/review", s.handleReviewPost)
				r.Post("/posts/{id}/flags", s.handleReviewFlags)
				r.Post("/polls/{id}/vote", s.handleVote)
				r.Post("/polls/{id}/close", s.handleClosePoll)

				// the other Pocket apps
				r.Get("/apps", s.handleApps)
				r.Get("/apps/chats", s.handleChats)
				r.Post("/apps/chat", s.handleShareChat)
				r.Post("/apps/email", s.handleShareEmail)
				r.Get("/apps/contacts", s.handleContacts)
				r.Post("/apps/calendar", s.handleToCalendar)
				r.Post("/apps/tasks", s.handleToTasks)
				r.Get("/apps/teams", s.handleTeams)
				r.Post("/apps/projects", s.handleToProjects)
				r.Get("/apps/notes", s.handleNotesSearch)
				r.Get("/apps/notes/{id}", s.handleNoteGet)
				r.Post("/apps/notes", s.handleToNotes)
				r.Post("/apps/files", s.handleToFiles)
				r.Get("/apps/photos", s.handlePhotosList)
				r.Get("/apps/photos/{uuid}/image", s.handlePhotoImage)
				r.Get("/apps/learn/decks", s.handleLearnDecks)
				r.Post("/apps/learn", s.handleToLearn)
				r.Post("/apps/blog", s.handleToBlog)
				r.Get("/apps/books", s.handleBooks)
				r.Get("/apps/places", s.handlePlaces)

				// AI
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)

				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)
				r.Get("/export.csv", s.handleExportCSV)
			})
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_forum_info Build info.\n# TYPE pocket_forum_info gauge\npocket_forum_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_forum_uptime_seconds Process uptime.\n# TYPE pocket_forum_uptime_seconds gauge\npocket_forum_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_forum_http_requests_total HTTP requests served.\n# TYPE pocket_forum_http_requests_total counter\npocket_forum_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_forum_http_errors_total HTTP 5xx responses.\n# TYPE pocket_forum_http_errors_total counter\npocket_forum_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_forum_rows Rows per table.\n# TYPE pocket_forum_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_forum_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
