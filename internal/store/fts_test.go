package store

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestFTS(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "x.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO post_search (rowid, title, body) VALUES (1, 'Kako podesiti ruter', 'Pozdrav svima, čuo sam da je Wi-Fi spor'), (2, '', 'the router is slow')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{FTSQuery("ruter", false), "title : (" + FTSQuery("ruter podesiti", true) + ")", FTSQuery("cuo", false), FTSQuery("rout", false)} {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM post_search WHERE post_search MATCH ?`, q).Scan(&n); err != nil {
			t.Fatal(q, err)
		}
		t.Log(q, n)
		if n != 1 {
			t.Error(q, n)
		}
	}
	if Slugify("Emir Kovač đak", "-") != "emir-kovac-djak" {
		t.Error(Slugify("Emir Kovač đak", "-"))
	}
}
