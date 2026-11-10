# Backup

Everything lives in `DATA_DIR`: the SQLite file `pocket-forum.db` (WAL mode; pictures are inside it). Take a consistent copy with `pocket-forum backup` (writes `DATA_DIR/backups/pocket-forum-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin). Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-forum.db` (remove the `-wal`/`-shm` files), start.

Every person can also export the forums they own, whole (categories, topics, posts, polls; authors by name), with their bookmarks and drafts, as JSON (`/api/v1/export.json`, back in with Settings → Backup & data — twice is the same as once), and every topic of their forums as CSV (`/api/v1/export.csv`). Pictures and files live in the database, so the SQLite backup has them.
