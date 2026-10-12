# Personal Expense Tracker — Project Scope

## Overview
A fullstack web application for tracking personal expenses with category breakdowns, budgets, and visualizations.

## Tech Stack
- **Frontend:** React
- **Backend:** Go (REST API)
- **Database:** Neon (PostgreSQL)
- **Auth:** Email/password with JWT in an httpOnly cookie

## Features

### Authentication
- Register with email + password
- Login with email + password
- Passwords must be at least 8 characters
- JWT-based session management. The JWT is stored in an httpOnly cookie named `session` (`SameSite=Lax`, `Secure` in production, 7-day lifetime), never in `localStorage` and never returned in a response body
- `POST /auth/logout` clears the cookie. `GET /auth/me` returns the current user, so the frontend can tell whether someone is logged in
- Login rate limit: after 5 failed logins for the same email, or 20 failed logins from the same client IP, within 15 minutes, further logins for that email or from that IP return `429` with a `Retry-After` header (seconds; the longer of the two when both apply), even with a correct password. A successful login resets the email counter only
- `TRUSTED_PROXY` (default off; only the exact value `true` enables it): the client IP for the limit is normally the connection's remote address and `X-Forwarded-For` is ignored. Enable it only behind a reverse proxy that appends the real client IP to `X-Forwarded-For`, otherwise clients can forge the header. When enabled and `TRUSTED_PROXIES` is unset or empty, the last entry of the last `X-Forwarded-For` line is used (the remote address is not checked); a missing or invalid value falls back to the remote address. Instead of an address list, `TRUSTED_PROXY_HOPS` can give the number of proxies, and `TRUSTED_PROXY_HEADER` can name another header to read instead of `X-Forwarded-For` (see below)
- `TRUSTED_PROXIES` (optional; only matters when `TRUSTED_PROXY=true`): comma-separated IPs and CIDRs, IPv4 or IPv6, spaces allowed, e.g. `10.0.0.0/8, 192.168.1.5, fd00::/8`. A bare IP matches only that address; IPv4-mapped IPv6 addresses (`::ffff:10.0.0.1`) count as the IPv4 address. Per request: if the remote address is not in the list, `X-Forwarded-For` is ignored and the client IP is the remote address. Otherwise all `X-Forwarded-For` lines are joined in order into one chain and walked from the right, skipping entries in the list; the first entry not in the list is the client IP. A malformed entry reached in the walk, an empty chain, or a chain where every entry is trusted falls back to the remote address. An invalid entry (bad CIDR, not an IP, empty entry, zone id) stops the server at startup with `invalid TRUSTED_PROXIES entry "<entry>"`; if the list is set while `TRUSTED_PROXY` is not `true`, the server starts and logs a warning that the list is ignored
- `TRUSTED_PROXY_HOPS` (optional; only matters when `TRUSTED_PROXY=true`): an integer from 1 to 64, spaces around it allowed; unset or empty means off and behavior is exactly as described for `TRUSTED_PROXIES` above. N is the number of proxies in front of the server, each appending the address it saw to `X-Forwarded-For`. All `X-Forwarded-For` lines are joined in order into one chain, split on commas, entries trimmed; the client IP is the Nth entry from the right, in normalized form (IPv4-mapped IPv6 shown as IPv4). Entries left of the Nth are forged or ignored and are not inspected. Examples: N=1, chain `1.1.1.1, 2.2.2.2` gives `2.2.2.2`; N=2 gives `1.1.1.1`; N=2, chain `9.9.9.9, 1.1.1.1, 2.2.2.2` gives `1.1.1.1`; N=2 with two header lines `1.1.1.1` and `2.2.2.2` gives `1.1.1.1`. A chain shorter than N (including a missing or empty header), or an Nth entry that is not a valid IP (garbage, empty, with a port or zone id), falls back to the remote address. The remote address is not checked against any list in this mode (limitation: only use it when the server is reachable only through the proxies). A value that is zero, negative, non-numeric, or above 64 stops the server at startup with `invalid TRUSTED_PROXY_HOPS "<value>"`. Setting it together with a non-empty `TRUSTED_PROXIES` stops the server at startup with a message that only one of the two may be set (checked even when `TRUSTED_PROXY` is not `true`). If it is set while `TRUSTED_PROXY` is not `true`, the server starts and logs a warning that the hop count is ignored
- `TRUSTED_PROXY_HEADER` (optional; only matters when `TRUSTED_PROXY=true`): the name of a single-value header that carries the real client IP, e.g. `X-Real-IP`, `CF-Connecting-IP`, `True-Client-IP`. Unset, empty or only spaces means the default, `X-Forwarded-For`, and behavior is exactly as described above; naming `X-Forwarded-For` (any case) is the same as unset. The name is trimmed and case-insensitive. With another header the client IP is read from it instead of `X-Forwarded-For`, which is then ignored entirely. The value (surrounding spaces trimmed) must be exactly one valid IP, in normalized form (IPv4-mapped IPv6 shown as IPv4); the header must appear on exactly one line. A missing or empty header, two header lines (ambiguous), or a value that is a list, has a port or a zone id, or is garbage falls back to the remote address, never to `X-Forwarded-For`. Together with `TRUSTED_PROXIES` the list only gates trust: the header is read only when the remote address is in the list, otherwise (and on any fallback) the client IP is the remote address; there is no chain walk. Without `TRUSTED_PROXIES` the remote address is not checked (limitation: only use it when the server is reachable only through the proxy). A name that is not an HTTP token (empty after trimming is fine; a space, colon, comma, control or non-ASCII character inside the name is not) stops the server at startup with `invalid TRUSTED_PROXY_HEADER "<value>"`, checked even when `TRUSTED_PROXY` is not `true`. Setting another header together with `TRUSTED_PROXY_HOPS` stops the server at startup with a message that only one of `TRUSTED_PROXY_HEADER` and `TRUSTED_PROXY_HOPS` may be set (also checked even when `TRUSTED_PROXY` is not `true`); `X-Forwarded-For` does not conflict with it. If a valid other header is set while `TRUSTED_PROXY` is not `true`, the server starts and logs a warning that the header setting is ignored, and the client IP is the remote address. Warning: the proxy must overwrite this header with the address it saw (not append to it or pass on the client's own value), or the operator must use a header the proxy always sets (such as `CF-Connecting-IP`); otherwise clients can forge their IP and dodge the limit
- In development, Vite proxies `/api` to the Go server (stripping the prefix), so the cookie is same-origin and no CORS setup is needed

### Expense Logging
- Log an expense with:
  - Amount
  - Category
  - Date (future dates are allowed, for planned or recurring costs; the pie chart and budget tracking only count the selected month)
  - Optional note

### Categories
- Default categories: Food, Transport, Housing, Entertainment, Health, Other
- Default categories are shared (`user_id` is null) and cannot be edited or deleted
- Category names are unique per user, case-insensitive, and may not match a default name (`409`)
- Users can create custom categories on top of defaults, and rename, recolor, or delete their own
- Deleting a custom category moves its expenses to the default "Other" category, then deletes the category and its budget rows
- Users can only change or delete their own categories (never another user's, never defaults)

### Budgets
- Per-category monthly budget (e.g., Food: 150,000 ₸/month)
- Overall monthly budget
- Budget vs. actual spending tracking
- One budget per user, category, and month. The overall budget (`category_id` null) is also unique per user and month. Enforce with a unique index that treats nulls as equal (`NULLS NOT DISTINCT`, Postgres 15+) so `PUT /budgets` can upsert

### Charts & Visualization
- Pie chart: spending breakdown by category for the current month

## Project Structure

### Backend (Go)
- JWT authentication middleware
- REST endpoints:
  - `GET /config` — public (no session, no database); returns `{"maxAmount": 1000000000000}`, the amount limit, read from the same constant the validation uses
  - `POST /auth/register`
  - `POST /auth/login` — sets the `session` cookie
  - `POST /auth/logout` — clears the cookie
  - `GET /auth/me` — current user (id, email)
  - `GET/POST /expenses` — `GET` takes an optional `?month=YYYY-MM` filter
  - `GET/PUT/DELETE /expenses/:id`
  - `GET/POST /categories`
  - `PUT/DELETE /categories/:id` — custom categories only; defaults return `403`
  - `GET/PUT /budgets` — the API uses `?month=YYYY-MM` (e.g. `2026-10`) and converts it to the first day of that month (`2026-10-01`) for storage
  - Ids in paths: a non-numeric id in `/expenses/:id` or `/categories/:id` returns `400`; a numeric id that does not exist (or belongs to another user) returns `404`
- Neon/PostgreSQL for data storage

### Frontend (React)
- Auth pages: Login / Register
- Dashboard: pie chart + monthly summary
- Expense list + add expense form
- Budget settings page
- Category management

## Money
- Single currency: Kazakhstani tenge (₸, KZT). No currency column, no multi-currency.
- Amounts are stored and sent over the API as whole tenge in a `BIGINT` (`1500` = 1,500 ₸). No fractional amounts (tiyn).
- Amounts must be positive integers of at most 1,000,000,000,000 (one trillion tenge), for expenses and budgets alike. The backend owns this limit; the frontend reads it once from `GET /config` and does not hard-code it. While that request is pending or if it fails, the frontend skips its own cap check and shows the backend's `400` message.
- The frontend formats with `Intl.NumberFormat('ru-KZ', { style: 'currency', currency: 'KZT' })`.
- Amount fields group digits by thousands with a regular space while typing (`1 500`); non-digits are left as typed so the validator rejects them on submit, and a hand-typed comma stays a comma. Defaults chosen by the orchestrator, not yet confirmed by the user (#34).
- Pasting a value with one `₸`/`KZT`/`тг` marker (before or after, case-insensitive, optional space) into an amount field drops the marker and groups the number (`1 500 ₸` -> `1 500`). Only pasting does this; a marker typed by hand is left for the validator to reject. Defaults chosen by the PM, not yet confirmed by the user (#35).

## Database Tables (Planned)
- `users` — id, email, password_hash, created_at
- `categories` — id, user_id (null = default), name, color
- `expenses` — id, user_id, category_id, amount (BIGINT, whole tenge), date, note, created_at
- `budgets` — id, user_id, category_id (null = overall), amount (BIGINT, whole tenge), month (DATE, always the 1st of the month, enforced by a CHECK constraint)

## Schema migrations
The schema lives in `backend/db/migrations/` as embedded `NNNN_description.sql` files (four-digit version, lowercase description). On startup `db.ApplySchema` runs the ones not yet recorded in the `schema_migrations` table, in version order, in one transaction under an advisory lock; a failure rolls everything back (except around a non-transactional migration, see below).

Adding a migration:
- Create the next file, e.g. `0002_add_expense_tags.sql`. Versions must be contiguous (no gaps, no duplicates).
- Never edit a migration that has been applied anywhere (startup fails with `modified after it was applied`); add a new one instead.
- Write plain SQL valid inside a transaction. A statement that cannot run in one (e.g. `CREATE INDEX CONCURRENTLY`) needs the non-transactional opt-out described below.
- A database whose `schema_migrations` has a version this binary lacks refuses to start (`newer than this binary`).

Reverting (down migrations):
- A migration may have a down file next to it, `NNNN_description.down.sql` (same version and description as the up file). Add a down file next to each new migration; it is optional, but a migration without one cannot be reverted. `0001` has none and cannot be reverted. A down file without a matching up file, or `0001_*.down.sql`, fails loading.
- Run `budgetcontrol migrate down` (reads only `DATABASE_URL`; it never starts the server). It reverts only the single latest applied migration: the down SQL and the deletion of its `schema_migrations` row run in one transaction under the same advisory lock as startup, and it prints `reverted <NNNN_name>`. Run it again to revert the next one. Several steps or a target version are not supported.
- It refuses, changing nothing, when the latest migration has no down file (`no down migration for <NNNN_name>`), when only `0001` is applied (`0001_initial cannot be reverted`), or on any consistency failure (`modified after it was applied`, `newer than this binary`, a hole). With nothing applied it prints `nothing to revert` and does not create `schema_migrations` or adopt a legacy database.
- Exit codes (all `migrate` verbs): 0 success (including `nothing to revert`), 1 failure (errors on stderr), 2 usage error (`budgetcontrol migrate` with a missing or unknown verb or extra arguments).
- The server never runs, reads or checksums down files, and the down file is not recorded, so it may be edited freely.
- Warning: starting the server after `down` applies that migration again. Deploy the older binary before reverting, or the next start undoes the revert.

Non-transactional migrations (`-- migrate:no-transaction`):
- A migration, up or down, opts out of the transaction with the exact first line `-- migrate:no-transaction` (LF or CRLF). The same text on any other line is an ordinary comment. The file name and checksum rules do not change (the marker is part of the checksummed bytes, so adding or removing it on an applied file gives `modified after it was applied`). `0001_initial.sql` cannot use it.
- The file should hold exactly one statement (comments and a trailing `;` are fine). The loader does not parse SQL; PostgreSQL rejects a multi-statement file that contains `CREATE INDEX CONCURRENTLY`, and the run fails naming the file.
- Order and grouping: pending migrations still run in version order. Consecutive transactional ones share one transaction; a non-transactional one first commits the open group and runs on its own, then the next group starts. `0002` transactional, `0003` non-transactional, `0004` transactional are three separate commits. A run with no non-transactional migration is unchanged (one transaction, all or nothing); otherwise atomicity is per group, so a failure leaves the earlier groups applied.
- Steps of one non-transactional migration: (1) a transaction creates `schema_migrations_in_progress` if needed (`version`, `name`, `direction` `up` or `down`, `started_at`; a database that never runs such a migration never gets the table) and writes the row, (2) the statement runs with no transaction, (3) a transaction inserts the `schema_migrations` row and deletes the in-progress row. Such a migration is not atomic.
- Lock: when the migration set contains a non-transactional file (for `migrate down`, also a non-transactional down file), the run takes the same advisory lock at session level on one connection, before any transaction, and holds it until the last migration finished, on success and on failure. A waiting process polls for the lock instead of blocking inside a transaction, because `CREATE INDEX CONCURRENTLY` waits for every older transaction and would otherwise deadlock with it.
- Failure: any failure of the statement (SQL error, cancelled context, lost connection, crash) leaves the in-progress row and writes no `schema_migrations` row; nothing after it runs. A failed `CREATE INDEX CONCURRENTLY` also leaves an invalid index. Startup, `migrate up` and `migrate down` then refuse to run anything (even earlier pending migrations) with `migration <file> did not finish (<up|down>): ...` and the exact statement `DELETE FROM schema_migrations_in_progress WHERE version = N`. Startup logs `apply schema: <err>` and exits non-zero. `migrate up` exits 1 and still prints `applied <NNNN_name>` for each migration that did commit before the failure. `migrate status` lists an interrupted `up` as `pending` / `interrupted` (counted as pending), names it on stderr with the same wording and exits 1; it creates nothing and does not say which migrations are non-transactional.
- Recovery is by hand: inspect the database (for example drop the invalid index), run the printed `DELETE`, run again; the migration is attempted again from the start.
- `migrate down`: the down file decides, whatever the up file is. With the marker the down statement runs outside a transaction (in-progress row with direction `down`, statement, then one transaction deletes the `schema_migrations` row and the in-progress row); without it, as before. A failed non-transactional down keeps both rows and the next run fails with `did not finish`. Write non-transactional down SQL so it can be repeated (`DROP INDEX CONCURRENTLY IF EXISTS`). All the refusals above (no down file, only `0001`, checksum, newer, hole) still happen before anything runs.

Status and apply-only (`budgetcontrol migrate status|up`, read only `DATABASE_URL`, never start the server):
- `migrate status` is read-only: it never creates `schema_migrations` or `schema_migrations_in_progress`, never adopts a legacy database, and takes no lock. It prints one tab-separated line per migration: version, description, `applied` or `pending`, checksum state (`ok`, `mismatch`, `-` for pending, `hole` for a pending migration below an applied one, `interrupted` for a non-transactional one that did not finish). A version in the database that this binary lacks is listed last as `applied` / `newer`. The last line is `<A> applied, <P> pending`.
- Pending migrations alone exit 0. A `mismatch`, `newer` or `hole` still prints the full listing, names each problem on stderr and exits 1. A legacy database (users, no `schema_migrations`) is listed all pending with a stderr note that `migrate up` will record `0001_initial` as applied.
- `migrate up` does what server startup does (`ApplyMigrations`: advisory lock, one transaction, legacy adoption, same checks). After the commit it prints `applied <NNNN_name>` per migration (`adopted 0001_initial` for a legacy database), or `nothing to apply`. On failure of a purely transactional run nothing is applied and nothing is printed on stdout; with a non-transactional migration, the migrations that did commit are still printed. Running it before deploying a new binary is optional, since startup applies the same migrations.

## Next Steps
1. Set up Neon database and schema
2. Build Go REST API with auth
3. Build React frontend
