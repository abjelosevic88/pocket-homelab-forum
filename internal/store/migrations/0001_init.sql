CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Forum ----------
-- A forum belongs to the person who made it (forums.user_id). Everyone else
-- reaches it as a member (members), or — for a public forum — reads it
-- through its link. Admins of the box have no way in to a forum they are not
-- a member of. Posts and topics keep author_name, so a guest's reply, an
-- imported post or someone whose account was removed still reads right
-- (their user_id becomes NULL instead of taking the thread down with them).
-- Times are UTC ISO strings; event times are local "YYYY-MM-DDTHH:MM".

-- How a person shows up in every forum: a handle for @mentions, a line about
-- them, an avatar colour or picture.
CREATE TABLE profiles (
  user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  handle     TEXT NOT NULL UNIQUE COLLATE NOCASE,
  bio        TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  avatar_id  INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE forums (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- the owner
  slug          TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  about         TEXT NOT NULL DEFAULT '',
  emoji         TEXT NOT NULL DEFAULT '💬',
  color         TEXT NOT NULL DEFAULT '',
  visibility    TEXT NOT NULL DEFAULT 'private',   -- private | box | public
  join_policy   TEXT NOT NULL DEFAULT 'invite',    -- open | approval | invite
  guest_posts   INTEGER NOT NULL DEFAULT 0,        -- public forums: people without an account may post (always held)
  approve_first INTEGER NOT NULL DEFAULT 0,        -- hold a new member's first N posts
  rules         TEXT NOT NULL DEFAULT '',
  welcome       TEXT NOT NULL DEFAULT '',
  slow_mode     INTEGER NOT NULL DEFAULT 0,        -- seconds between two posts of one person
  edit_minutes  INTEGER NOT NULL DEFAULT 0,        -- 0 = members edit their posts any time
  blocked_words TEXT NOT NULL DEFAULT '[]',
  held_words    TEXT NOT NULL DEFAULT '[]',
  reactions     TEXT NOT NULL DEFAULT '[]',
  archived      INTEGER NOT NULL DEFAULT 0,
  source        TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX forums_user ON forums(user_id);

CREATE TABLE members (
  forum_id       INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role           TEXT NOT NULL DEFAULT 'member',   -- owner | moderator | member
  status         TEXT NOT NULL DEFAULT 'active',   -- active | pending | banned
  title          TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',         -- what they wrote asking to join
  level          TEXT NOT NULL DEFAULT 'normal',   -- new topics: watching | tracking | normal | muted
  digest         INTEGER NOT NULL DEFAULT 1,       -- in the person's summary email
  banned_until   TEXT,
  ban_reason     TEXT NOT NULL DEFAULT '',
  silenced_until TEXT,
  invited_by     INTEGER,
  posts_n        INTEGER NOT NULL DEFAULT 0,
  topics_n       INTEGER NOT NULL DEFAULT 0,
  likes_given    INTEGER NOT NULL DEFAULT 0,
  likes_got      INTEGER NOT NULL DEFAULT 0,
  solutions_n    INTEGER NOT NULL DEFAULT 0,
  joined_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_seen_at   TEXT,
  last_post_at   TEXT,
  PRIMARY KEY (forum_id, user_id)
);
CREATE INDEX members_user ON members(user_id, status);

CREATE TABLE invites (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  forum_id   INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- who made it
  token      TEXT NOT NULL UNIQUE,
  note       TEXT NOT NULL DEFAULT '',
  email      TEXT NOT NULL DEFAULT '',
  role       TEXT NOT NULL DEFAULT 'member',
  max_uses   INTEGER NOT NULL DEFAULT 0,          -- 0 = any number
  uses       INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  accounts   INTEGER NOT NULL DEFAULT 1,          -- someone without an account may make one with it
  revoked    INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX invites_forum ON invites(forum_id);

CREATE TABLE categories (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  forum_id   INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  parent_id  INTEGER REFERENCES categories(id) ON DELETE SET NULL,
  name       TEXT NOT NULL,
  slug       TEXT NOT NULL,
  about      TEXT NOT NULL DEFAULT '',
  emoji      TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  starts     TEXT NOT NULL DEFAULT 'all',         -- who starts topics: all | mods
  qa         INTEGER NOT NULL DEFAULT 0,          -- new topics are questions that get an answer
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (forum_id, slug)
);

CREATE TABLE topics (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  forum_id       INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  category_id    INTEGER REFERENCES categories(id) ON DELETE SET NULL,
  user_id        INTEGER REFERENCES users(id) ON DELETE SET NULL,
  author_name    TEXT NOT NULL DEFAULT '',
  title          TEXT NOT NULL,
  kind           TEXT NOT NULL DEFAULT 'discussion', -- discussion | question | announcement | event
  state          TEXT NOT NULL DEFAULT 'visible',    -- visible | pending | hidden | deleted
  closed         INTEGER NOT NULL DEFAULT 0,
  archived       INTEGER NOT NULL DEFAULT 0,
  pinned         INTEGER NOT NULL DEFAULT 0,         -- 0 | 1 in its category | 2 on the forum's front page
  pinned_until   TEXT,
  unlisted       INTEGER NOT NULL DEFAULT 0,
  solved_post_id INTEGER,
  event_start    TEXT NOT NULL DEFAULT '',
  event_end      TEXT NOT NULL DEFAULT '',
  event_place    TEXT NOT NULL DEFAULT '',
  close_at       TEXT,
  slow_mode      INTEGER NOT NULL DEFAULT 0,
  moved_to       INTEGER,                            -- merged into this topic
  tags           TEXT NOT NULL DEFAULT '[]',
  excerpt        TEXT NOT NULL DEFAULT '',
  views          INTEGER NOT NULL DEFAULT 0,
  posts_n        INTEGER NOT NULL DEFAULT 0,         -- visible posts, the first one included
  highest        INTEGER NOT NULL DEFAULT 0,         -- the highest post number given out
  likes_n        INTEGER NOT NULL DEFAULT 0,
  posters        TEXT NOT NULL DEFAULT '[]',
  last_post_at   TEXT,
  last_user_id   INTEGER,
  last_name      TEXT NOT NULL DEFAULT '',
  bumped_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  source         TEXT NOT NULL DEFAULT '',
  source_ref     TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX topics_forum ON topics(forum_id, state, bumped_at);
CREATE INDEX topics_category ON topics(category_id, bumped_at);
CREATE INDEX topics_user ON topics(user_id, created_at);
CREATE INDEX topics_source ON topics(forum_id, source, source_ref);

CREATE TABLE topic_tags (
  topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  forum_id INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  tag      TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY (topic_id, tag)
);
CREATE INDEX topic_tags_forum ON topic_tags(forum_id, tag);

CREATE TABLE posts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  topic_id      INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  forum_id      INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
  author_name   TEXT NOT NULL DEFAULT '',
  number        INTEGER NOT NULL,
  reply_to      INTEGER NOT NULL DEFAULT 0,          -- the post number it answers
  body          TEXT NOT NULL DEFAULT '',
  state         TEXT NOT NULL DEFAULT 'visible',     -- visible | pending | hidden | deleted
  hidden_reason TEXT NOT NULL DEFAULT '',
  wiki          INTEGER NOT NULL DEFAULT 0,
  edits         INTEGER NOT NULL DEFAULT 0,
  edited_at     TEXT,
  likes_n       INTEGER NOT NULL DEFAULT 0,
  reactions     TEXT NOT NULL DEFAULT '{}',
  guest_email   TEXT NOT NULL DEFAULT '',
  source        TEXT NOT NULL DEFAULT '',
  source_ref    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (topic_id, number)
);
CREATE INDEX posts_user ON posts(user_id, created_at);
CREATE INDEX posts_forum ON posts(forum_id, state, created_at);

CREATE VIRTUAL TABLE post_search USING fts5(title, body, tokenize = 'unicode61 remove_diacritics 2');

CREATE TABLE revisions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  editor     TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  reason     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX revisions_post ON revisions(post_id);

CREATE TABLE reactions (
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  emoji      TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (post_id, user_id, emoji)
);
CREATE INDEX reactions_user ON reactions(user_id);

-- What a person has read of a topic and how closely they follow it.
CREATE TABLE reads (
  topic_id   INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  last_read  INTEGER NOT NULL DEFAULT 0,
  level      TEXT NOT NULL DEFAULT '',             -- '' = from the category / forum
  visited_at TEXT,
  PRIMARY KEY (topic_id, user_id)
);
CREATE INDEX reads_user ON reads(user_id);

CREATE TABLE category_levels (
  category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  level       TEXT NOT NULL,
  PRIMARY KEY (category_id, user_id)
);

CREATE TABLE bookmarks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  note       TEXT NOT NULL DEFAULT '',
  remind_at  TEXT,
  reminded   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, post_id)
);

CREATE TABLE drafts (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key        TEXT NOT NULL,                        -- new:<forum id> | reply:<topic id> | edit:<post id>
  data       TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, key)
);

CREATE TABLE notifications (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  forum_id   INTEGER REFERENCES forums(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL,     -- mention | reply | quote | watching | new_topic | reaction | solved | join_request | joined | flag | pending | moderation | reminder | welcome
  topic_id   INTEGER,
  post_id    INTEGER,
  number     INTEGER NOT NULL DEFAULT 0,
  actor_id   INTEGER,
  actor_name TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  excerpt    TEXT NOT NULL DEFAULT '',
  read_at    TEXT,
  pushed     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX notifications_user ON notifications(user_id, read_at, id);

CREATE TABLE flags (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  forum_id   INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- who flagged it
  reason     TEXT NOT NULL,     -- spam | rude | off_topic | other
  note       TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'open',  -- open | agreed | disagreed | ignored
  handled_by INTEGER,
  handled_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX flags_forum ON flags(forum_id, status);

CREATE TABLE mod_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  forum_id    INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
  actor_name  TEXT NOT NULL DEFAULT '',
  action      TEXT NOT NULL,
  target_kind TEXT NOT NULL DEFAULT '',
  target_id   INTEGER NOT NULL DEFAULT 0,
  detail      TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX mod_log_forum ON mod_log(forum_id, id);

CREATE TABLE polls (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  post_id    INTEGER NOT NULL UNIQUE REFERENCES posts(id) ON DELETE CASCADE,
  topic_id   INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  forum_id   INTEGER NOT NULL REFERENCES forums(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  question   TEXT NOT NULL,
  options    TEXT NOT NULL DEFAULT '[]',
  multi      INTEGER NOT NULL DEFAULT 0,
  public     INTEGER NOT NULL DEFAULT 0,  -- show who voted for what
  closes_at  TEXT,
  closed     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE votes (
  poll_id    INTEGER NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  choice     INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (poll_id, user_id, choice)
);

-- Event topics: who is coming.
CREATE TABLE rsvps (
  topic_id   INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status     TEXT NOT NULL,   -- going | maybe | no
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (topic_id, user_id)
);

-- Pictures and files in posts, and avatars. The browser shrinks pictures first.
CREATE TABLE uploads (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  forum_id   INTEGER REFERENCES forums(id) ON DELETE CASCADE,  -- NULL = an avatar
  name       TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL DEFAULT 'application/octet-stream',
  bytes      INTEGER NOT NULL DEFAULT 0,
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  data       BLOB NOT NULL,
  thumb      BLOB,
  used       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX uploads_user ON uploads(user_id);
CREATE INDEX uploads_forum ON uploads(forum_id);
