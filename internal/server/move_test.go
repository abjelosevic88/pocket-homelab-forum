package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-forum/internal/config"
)

// moveLinks starts Forum behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "forum:read"}[parts[1]]
		email := parts[0] + "@example.com"
		name := strings.ToUpper(parts[0][:1]) + parts[0][1:]
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"forum","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, name)
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) raw(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) moveImport(tok, path string, data []byte) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "forum.json")
	_, _ = fw.Write(data)
	_ = mw.Close()
	code, out := a.raw(tok, "POST", path, mw.FormDataContentType(), buf.Bytes())
	if code != 200 {
		a.t.Fatalf("POST %s → %d %s", path, code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/home", 403},
		{exp, "GET", "/api/v1/review", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/forums", 403},
		{imp, "POST", "/api/v1/topics", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
		{read, "POST", "/api/v1/import", 403},
	} {
		if code, body := a.raw(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.raw(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 1 || p.Imports[0].Form != "file" || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move copies Alice's own forum out and brings it into Bob's account: each
// import adds once, a forum Bob is only a member of is not his to take, and
// Alice bringing her own file back into this box adds nothing.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	slug := a.makeForum("Book club", "box", "open")
	tid, _ := a.topic(a.alice, slug, "Next book", "Suggestions please")
	a.ok(a.bob, "POST", "/api/v1/forums/"+slug+"/join", nil)
	a.ok(a.bob, "POST", fmt.Sprintf("/api/v1/topics/%d/posts", tid), map[string]any{"body": "The Hobbit"})

	code, export := a.raw("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "Next book") {
		t.Fatalf("alice's export: %d %s", code, export)
	}
	if _, bob := a.raw("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(bob), "Next book") {
		t.Fatalf("bob's export holds alice's forum: %s", bob)
	}

	first := a.moveImport("pcx_bob_import", "/api/v1/import", export)
	if num(first, "forums") != 1 || num(first, "topics") != 1 || num(first, "posts") != 2 {
		t.Fatalf("first import: %v", first)
	}
	second := a.moveImport("pcx_bob_import", "/api/v1/import", export)
	if num(second, "forums")+num(second, "topics")+num(second, "posts") != 0 {
		t.Fatalf("second import added: %v", second)
	}
	back := a.moveImport("pcx_alice_import", "/api/v1/import", export)
	if num(back, "forums")+num(back, "topics")+num(back, "posts") != 0 {
		t.Fatalf("alice's own file back into this box added: %v", back)
	}

	owned := func(uid int64) int {
		var n int
		_ = a.st.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM forums WHERE user_id = ?`, uid).Scan(&n)
		return n
	}
	if owned(a.ids["Bob"]) != 1 || owned(a.ids["Alice"]) != 1 {
		t.Fatalf("forums: bob %d, alice %d", owned(a.ids["Bob"]), owned(a.ids["Alice"]))
	}
	var topics int
	_ = a.st.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM topics`).Scan(&topics)
	if topics != 2 {
		t.Fatalf("topics on the box: %d, want 2", topics)
	}
}
