# API

Everything the web app does goes through `/api/v1`. Authenticate with a session cookie, a personal token (`Authorization: Bearer pfrm_…`, Settings → API & shortcuts) or a Pocket Core link token. What you can read follows the forums: a private forum answers 404 to anyone who is not in it (admins of the box too); a box forum is readable by every account; a public forum also through `/api/v1/public/…` without an account.

## For you

| | |
|---|---|
| `GET /home` | Your profile, your forums with unread/new counts, and the numbers for notifications, bookmarks, drafts and review |
| `GET /summary` | Unread and new topics, notifications, the latest topics, events and open questions — for Homepage, Pocket AI, Pocket Day |
| `GET /day/{YYYY-MM-DD}` | What you wrote that day (Pocket Day) |
| `GET /search?q=` (`forum, category, tag, author, kind, solved=yes|no, titles=1`) | Full-text search, highest matches first, snippets with `⟦…⟧` around the words found |
| `GET /similar?forum=&title=` | Topics like a title being written |
| `GET /events` | Event topics coming up, with your answer |
| `GET /notifications` (`?unread=1`), `POST /notifications/read` `{"ids": []}` | Your notifications; no ids = all read |
| `GET /bookmarks`, `GET/PUT/DELETE /drafts/{new:ID|reply:ID|edit:ID}`, `GET /drafts` | Saved posts and unfinished drafts |
| `PUT /profile` `{"handle","bio","color","avatar_id"}`, `GET /people/{id|handle}` | Your forum profile; someone's page (forums you share with them) |
| `GET /stream` | Server-sent events: `post`, `topic`, `edit`, `reaction`, `poll`, `rsvp`, `notification`, `review`, `forum` |

## Forums

| | |
|---|---|
| `GET /forums` | Yours, the ones you could join, the starting templates |
| `POST /forums` | `{"name", "about", "emoji", "color", "visibility": "private|box|public", "join_policy": "open|approval|invite", "template"}` |
| `GET/PUT/DELETE /forums/{slug}` | A forum with its categories, staff, numbers and your membership; moderators change it, the owner decides who may see and join it |
| `POST /forums/{slug}/join` `{"note"}`, `POST /forums/{slug}/leave`, `PUT /forums/{slug}/me` `{"level","digest"}` | |
| `GET /forums/{slug}/members` (`status, q`), `POST /forums/{slug}/members` `{"email","role"}` | People; add someone with an account here |
| `PUT /forums/{slug}/members/{uid}` | `{"role"}`, `{"title"}`, `{"approve": true|false}`, `{"status": "banned", "ban_days", "ban_reason"}`, `{"silence_hours"}` |
| `GET/POST /forums/{slug}/invites`, `DELETE /invites/{id}`, `POST /invites/{token}/accept` | Invite links |
| `GET/POST /forums/{slug}/categories`, `PUT /forums/{slug}/categories/order`, `PUT/DELETE /categories/{id}` (`?move_to=`), `PUT /categories/{id}/level` | |
| `GET /forums/{slug}/tags`, `GET /forums/{slug}/people?q=`, `GET /forums/{slug}/log` | Tags in use, @mention suggestions, the moderation log |
| `POST /forums/{slug}/import/discourse` `{"url","limit"}` | Bring the newest topics of a public Discourse forum over (again = only what is new) |

## Topics and posts

| | |
|---|---|
| `GET /topics` (`forum, category, tag, kind, author, filter=latest|unread|new|top|unsolved|solved|noreplies|mine|posted|events, period, page`) | Lists with your read state |
| `POST /topics` | `{"forum", "category_id", "title", "body", "kind", "tags", "event_start", "event_end", "event_place", "poll": {"question","options","multi","public","days"}, "source_ref"}` |
| `GET/PUT/DELETE /topics/{id}` | A topic with its posts |
| `POST /topics/{id}/status` | Moderators: `closed, archived, pinned (0–2), pinned_days, unlisted, close_in_days, slow_mode, state` |
| `POST /topics/{id}/merge` `{"into"}`, `POST /topics/{id}/split` `{"post_ids","title"|"into"}` | |
| `POST /topics/{id}/read` `{"number"}`, `PUT /topics/{id}/level`, `POST /topics/{id}/solve` `{"post_id"}`, `POST /topics/{id}/rsvp` `{"status"}` | |
| `POST /topics/{id}/posts` `{"body","reply_to"}` | A reply |
| `GET/PUT/DELETE /posts/{id}`, `POST /posts/{id}/restore`, `GET /posts/{id}/revisions` | |
| `POST /posts/{id}/react` `{"emoji"}`, `GET /posts/{id}/reactions`, `POST/DELETE /posts/{id}/bookmark`, `POST /posts/{id}/flag`, `POST /posts/{id}/wiki` | |
| `POST /posts/{id}/hide`, `POST /posts/{id}/review` `{"action": "approve|reject"}`, `POST /posts/{id}/flags` `{"action": "agree|disagree|ignore|delete"}`, `GET /review` | Moderators |
| `POST /polls/{id}/vote` `{"choices"}`, `POST /polls/{id}/close` | |
| `POST /uploads` (multipart `file`, `forum`), `GET /uploads/{id}` | Pictures and files |

Posts are Markdown, plus `@handle` mentions and `[quote=handle #3]…[/quote]` quotes.

## Public (no account)

`GET /public/forums/{slug}`, `GET /public/forums/{slug}/topics`, `GET /public/topics/{id}`, `POST /public/forums/{slug}/topics` and `POST /public/topics/{id}/posts` (guests, held for a moderator), `GET /public/invites/{token}`, `POST /public/invites/{token}/signup`, and the feed `GET /feeds/{slug}.rss` (`?category=`).

## AI, other apps, data

`POST /ai/summary`, `/ai/reply`, `/ai/improve`, `/ai/suggest`, `/ai/ask`, `/ai/check`; `GET /apps` and `/apps/…` for Chat, Email, Contacts, Calendar, Tasks, Projects, Notes, Files, Photos, Learn, Blog, Bookshelf, Places, Translate; `GET /export.json`, `GET /export.csv`, `POST /import`, `GET /backup` (admin).

Pocket Move can copy your forums, bookmarks and drafts out and bring forums you owned elsewhere in (`GET /pocket/portable`, link scopes `move:export` / `move:import`); importing the same file twice adds nothing.
