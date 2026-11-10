package server

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

func contextBG() context.Context { return context.Background() }

// Posts are Markdown with two additions: [quote=handle #N]…[/quote] for a
// quoted post and @handle for a mention. These helpers read them on the
// server (notifications, excerpts, emails, feeds).

var (
	reMention  = regexp.MustCompile(`(^|[^\w@/.])@([a-zA-Z0-9][a-zA-Z0-9_.-]{1,29})`)
	reQuote    = regexp.MustCompile(`\[quote=([a-zA-Z0-9_.-]+)(?:\s+#(\d+))?\]`)
	reFence    = regexp.MustCompile("(?s)```.*?```")
	reInline   = regexp.MustCompile("`[^`\n]*`")
	reUploadID = regexp.MustCompile(`/api/v1/uploads/(\d+)`)
	reImage    = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	reLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reQuoteBlk = regexp.MustCompile(`(?s)\[quote=[^\]]*\].*?\[/quote\]`)
	reMarks    = regexp.MustCompile(`[*_~#>]+`)
	reSpaces   = regexp.MustCompile(`\s+`)
)

func withoutCode(body string) string {
	return reInline.ReplaceAllString(reFence.ReplaceAllString(body, " "), " ")
}

// mentions are the @handles in a post (outside code and quotes), once each.
func mentions(body string) []string {
	clean := reQuoteBlk.ReplaceAllString(withoutCode(body), " ")
	seen := map[string]bool{}
	var out []string
	for _, m := range reMention.FindAllStringSubmatch(clean, 30) {
		h := strings.ToLower(strings.TrimRight(m[2], "."))
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// quoted are the handles and post numbers a post quotes.
func quoted(body string) map[string]int {
	out := map[string]int{}
	for _, m := range reQuote.FindAllStringSubmatch(withoutCode(body), 20) {
		n, _ := strconv.Atoi(m[2])
		out[strings.ToLower(m[1])] = n
	}
	return out
}

// uploadIDs are the uploads a post points at.
func uploadIDs(body string) []int64 {
	var out []int64
	for _, m := range reUploadID.FindAllStringSubmatch(body, 100) {
		if id, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// plainText is a post without its Markdown, quotes and code, for excerpts.
func plainText(body string) string {
	t := reQuoteBlk.ReplaceAllString(body, " ")
	t = reFence.ReplaceAllString(t, " [code] ")
	t = reImage.ReplaceAllString(t, " [picture] ")
	t = reLink.ReplaceAllString(t, "$1")
	t = strings.ReplaceAll(t, "`", "")
	t = reMarks.ReplaceAllString(t, " ")
	return strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
}

func excerpt(body string, n int) string { return clip(plainText(body), n) }

// containsWord reports the first word of a list that appears in a text
// (whole words, any case; a word may be a phrase).
func containsWord(text string, words []string) string {
	low := " " + strings.ToLower(reSpaces.ReplaceAllString(regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(text, " "), " ")) + " "
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		if strings.Contains(low, " "+w+" ") {
			return w
		}
	}
	return ""
}

// mdHTML renders a post's Markdown as simple, safe HTML for emails and
// feeds: paragraphs, line breaks, bold, italic, code, links, pictures,
// quotes and lists. Links are absolute (base) and only http(s).
func mdHTML(body, base string) string {
	abs := func(u string) string {
		u = strings.TrimSpace(u)
		if strings.HasPrefix(u, "/") {
			return strings.TrimRight(base, "/") + u
		}
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "mailto:") {
			return u
		}
		return ""
	}
	inline := func(t string) string {
		t = html.EscapeString(t)
		t = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`).ReplaceAllStringFunc(t, func(m string) string {
			p := regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`).FindStringSubmatch(m)
			if u := abs(html.UnescapeString(p[2])); u != "" {
				return fmt.Sprintf(`<img src="%s" alt="%s" style="max-width:100%%">`, html.EscapeString(u), p[1])
			}
			return p[1]
		})
		t = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`).ReplaceAllStringFunc(t, func(m string) string {
			p := regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`).FindStringSubmatch(m)
			if u := abs(html.UnescapeString(p[2])); u != "" {
				return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(u), p[1])
			}
			return p[1]
		})
		t = regexp.MustCompile("`([^`]+)`").ReplaceAllString(t, "<code>$1</code>")
		t = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(t, "<b>$1</b>")
		t = regexp.MustCompile(`(^|[\s(])[*_]([^*_\s][^*_]*)[*_]`).ReplaceAllString(t, "$1<i>$2</i>")
		return t
	}
	var out strings.Builder
	lines := strings.Split(strings.ReplaceAll(body, "\r", ""), "\n")
	var para []string
	flush := func() {
		if len(para) > 0 {
			out.WriteString("<p>" + strings.Join(para, "<br>") + "</p>\n")
			para = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		tl := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(tl, "```"):
			flush()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			out.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>\n")
		case reQuote.MatchString(tl) && strings.HasPrefix(tl, "[quote="):
			flush()
			m := reQuote.FindStringSubmatch(tl)
			rest := strings.TrimSpace(tl[len(m[0]):])
			var q []string
			if rest != "" {
				q = append(q, rest)
			}
			for i++; i < len(lines) && !strings.Contains(lines[i], "[/quote]"); i++ {
				q = append(q, lines[i])
			}
			if i < len(lines) {
				if before := strings.TrimSpace(strings.SplitN(lines[i], "[/quote]", 2)[0]); before != "" {
					q = append(q, before)
				}
			}
			out.WriteString(`<blockquote style="border-left:3px solid #ccc;margin:0 0 8px;padding:4px 10px;color:#555"><b>` + html.EscapeString(m[1]) + `</b>: ` + inline(strings.Join(q, " ")) + "</blockquote>\n")
		case strings.HasPrefix(tl, ">"):
			flush()
			var q []string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">"); i++ {
				q = append(q, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")))
			}
			i--
			out.WriteString(`<blockquote style="border-left:3px solid #ccc;margin:0 0 8px;padding:4px 10px;color:#555">` + inline(strings.Join(q, " ")) + "</blockquote>\n")
		case regexp.MustCompile(`^([-*•]|\d+\.)\s+`).MatchString(tl):
			flush()
			out.WriteString("<ul>")
			for ; i < len(lines) && regexp.MustCompile(`^\s*([-*•]|\d+\.)\s+`).MatchString(lines[i]); i++ {
				out.WriteString("<li>" + inline(regexp.MustCompile(`^\s*([-*•]|\d+\.)\s+`).ReplaceAllString(lines[i], "")) + "</li>")
			}
			i--
			out.WriteString("</ul>\n")
		case strings.HasPrefix(tl, "#"):
			flush()
			out.WriteString("<p><b>" + inline(strings.TrimSpace(strings.TrimLeft(tl, "#"))) + "</b></p>\n")
		case tl == "":
			flush()
		default:
			para = append(para, inline(tl))
		}
	}
	flush()
	return out.String()
}

// mdPlain is a post as plain text for an email or a note (quotes as "> ").
func mdPlain(body string) string {
	b := reQuote.ReplaceAllString(body, "> $1 wrote:\n>")
	return strings.ReplaceAll(b, "[/quote]", "")
}
