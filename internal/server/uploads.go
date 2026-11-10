package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-forum/internal/store"
)

const maxUpload = 12 << 20

var allowedMime = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true, "application/pdf": true, "text/plain": true,
	"application/zip": true, "text/csv": true, "audio/mpeg": true, "audio/mp4": true, "video/mp4": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	"application/vnd.oasis.opendocument.text": true, "application/msword": true, "application/vnd.ms-excel": true}

// handleUpload stores a picture (shrunk by the browser) or a file for a
// post in a forum (or, without a forum, the person's avatar).
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "a file can be up to 12 MB")
		return
	}
	up := &store.Upload{UserID: u.ID}
	if ref := r.FormValue("forum"); ref != "" {
		a, err := s.forumAccess(ctx, u, ref)
		if err != nil || !a.member() {
			writeErr(w, http.StatusForbidden, "join the forum to add files")
			return
		}
		if ok, why := a.canPost(); !ok {
			writeErr(w, http.StatusForbidden, why)
			return
		}
		up.ForumID = a.f.ID
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil || len(data) > maxUpload {
		writeErr(w, http.StatusBadRequest, "a file can be up to 12 MB")
		return
	}
	mime := http.DetectContentType(data)
	if i := strings.Index(mime, ";"); i > 0 {
		mime = mime[:i]
	}
	if mime == "application/octet-stream" || mime == "text/plain" || mime == "application/zip" {
		if guess := mimeByExt(hdr.Filename); guess != "" {
			mime = guess
		}
	}
	if !allowedMime[mime] {
		writeErr(w, http.StatusBadRequest, "pictures, PDFs, documents, spreadsheets, text, zip, audio and video files only")
		return
	}
	if up.ForumID == 0 && !strings.HasPrefix(mime, "image/") {
		writeErr(w, http.StatusBadRequest, "an avatar is a picture")
		return
	}
	up.Mime, up.Data, up.Name = mime, data, safeName(hdr.Filename)
	up.Width, _ = strconv.Atoi(r.FormValue("width"))
	up.Height, _ = strconv.Atoi(r.FormValue("height"))
	if tf, _, err := r.FormFile("thumb"); err == nil {
		up.Thumb, _ = io.ReadAll(io.LimitReader(tf, 1<<20))
		tf.Close()
	}
	if err := s.store.SaveUpload(ctx, up); err != nil {
		s.fail(w, err, "upload")
		return
	}
	url := fmt.Sprintf("/api/v1/uploads/%d", up.ID)
	md := fmt.Sprintf("[%s](%s)", up.Name, url)
	if strings.HasPrefix(mime, "image/") {
		md = fmt.Sprintf("![%s](%s)", strings.TrimSuffix(up.Name, filepath.Ext(up.Name)), url)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"upload": up, "url": url, "markdown": md})
}

func mimeByExt(name string) string {
	return map[string]string{".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/plain", ".csv": "text/csv", ".zip": "application/zip", ".mp3": "audio/mpeg",
		".m4a": "audio/mp4", ".mp4": "video/mp4", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".odt": "application/vnd.oasis.opendocument.text", ".doc": "application/msword", ".xls": "application/vnd.ms-excel"}[strings.ToLower(filepath.Ext(name))]
}

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*[]()`, r) {
			return '_'
		}
		return r
	}, n)
	return firstNonEmpty(clip(strings.TrimSpace(n), 120), "file")
}

// mayGetUpload: the uploader; anyone who can read its forum; for an
// avatar, anyone who shares a forum with its owner (or, public = true,
// when its owner is in a public forum).
func (s *Server) mayGetUpload(ctx context.Context, u *store.User, up *store.Upload) bool {
	if u != nil && up.UserID == u.ID {
		return true
	}
	if up.ForumID > 0 {
		f, err := s.store.GetForum(ctx, up.ForumID)
		if err != nil {
			return false
		}
		a, err := s.accessTo(ctx, u, f)
		return err == nil && a.canRead()
	}
	if u != nil && len(s.store.SharedForums(ctx, u.ID, up.UserID)) > 0 {
		return true
	}
	var n int
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM members m JOIN forums f ON f.id = m.forum_id WHERE m.user_id = ? AND m.status = 'active' AND f.visibility IN ('public', 'box')`, up.UserID).Scan(&n)
	return n > 0
}

func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request, u *store.User) {
	ctx := r.Context()
	up, err := s.store.GetUpload(ctx, idParam(r, "id"))
	if err != nil || !s.mayGetUpload(ctx, u, up) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	data := up.Data
	if queryBool(r, "thumb") && len(up.Thumb) > 0 {
		data = up.Thumb
	}
	w.Header().Set("Content-Type", up.Mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disp := "inline"
	if !strings.HasPrefix(up.Mime, "image/") && up.Mime != "application/pdf" && !strings.HasPrefix(up.Mime, "audio/") && !strings.HasPrefix(up.Mime, "video/") {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, strings.ReplaceAll(up.Name, `"`, "")))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	_, _ = w.Write(data)
}

func (s *Server) handleGetUpload(w http.ResponseWriter, r *http.Request) {
	s.serveUpload(w, r, userFrom(r.Context()))
}

func (s *Server) handlePublicUpload(w http.ResponseWriter, r *http.Request) {
	u, _ := s.resolveUser(r)
	s.serveUpload(w, r, u)
}
