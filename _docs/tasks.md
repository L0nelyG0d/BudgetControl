# BudgetControl — Task Backlog

## 1. Go backend skeleton with a passing test
Goal: Scaffold an empty Go project that compiles and has one passing test.
Description: Initialize a Go module under `backend/`, add a minimal HTTP server that responds to `GET /health` with `200 OK`, and write one test that asserts that response. No database or business logic yet — just a working, testable foundation.

## 2. React frontend skeleton with a passing test
Goal: Scaffold an empty React app (Vite) that renders and has one passing test.
Description: Create the project under `frontend/` using Vite with the React + TypeScript template. Set up Tailwind CSS v4 and run `shadcn init` with the indigo theme and CSS variables, following `_docs/design-system.md` (light and dark tokens in `src/index.css`). Replace the default page with a bare `<App />` component that renders a heading, and add one Vitest/Testing Library test that asserts the heading is present. Configure the Vite dev server to proxy `/api` to `http://localhost:8080`, stripping the prefix, so the session cookie is same-origin. No routing, API calls, or other shadcn components yet.

## 3. Neon database schema
Goal: Create the PostgreSQL schema on Neon with all four tables.
Description: Using the Neon console or migration SQL file, create `users`, `categories`, `expenses`, and `budgets` tables matching the schema in `_docs/plan.md`. Amounts are `BIGINT` whole tenge, `budgets.month` is a `DATE` with a `CHECK` that it is the 1st of the month, and `budgets` has a unique index on (user_id, category_id, month) using `NULLS NOT DISTINCT` (Postgres 15+). Seed the `categories` table with the default categories (Food, Transport, Housing, Entertainment, Health, Other). Emails are unique case-insensitively, category names are unique per user (case-insensitive), and amounts must be positive. Add an index on `expenses (user_id, date)`. Commit the SQL file to `backend/db/schema.sql`.

## 4. Backend: user registration endpoint
Goal: Implement `POST /auth/register` that creates a new user.
Description: Accept `email` and `password` in the request body, hash the password with bcrypt, insert a row into `users`, and return `201` with the new user's id and email. Emails are trimmed and lowercased. Reject passwords shorter than 8 characters with `400`. Return `409` if the email is already taken. This task also adds the `DATABASE_URL` connection (pgx pool) and the JSON `{"error": ...}` error format reused by later endpoints. Write a test that covers the success, duplicate-email, and short-password cases.

## 5. Backend: login endpoint and JWT middleware
Goal: Implement login, logout, and current-user endpoints, and protect routes with a JWT middleware.
Description: `POST /auth/login` verifies the email/password against the database, signs a JWT containing the user id and a 7-day expiry, and sets it in an httpOnly `session` cookie (`SameSite=Lax`, `Secure` in production). The token is never returned in the response body. `POST /auth/logout` clears the cookie. `GET /auth/me` returns the current user's id and email. Add middleware that reads the `session` cookie, validates the JWT, and attaches the user id to the request context. The signing secret comes from the `JWT_SECRET` environment variable. Write tests for valid login (cookie set), wrong password, logout, `/auth/me`, and a protected route accessed without a cookie.

## 6. Backend: categories endpoints
Goal: Implement full category management behind auth.
Description: `GET /categories` returns the authenticated user's custom categories merged with the global defaults (where `user_id IS NULL`). `POST /categories` creates a custom category (name + color). `PUT /categories/:id` renames or recolors a custom category. `DELETE /categories/:id` moves the category's expenses to the default "Other" category, then deletes the category and its budget rows (in one transaction). Default categories and other users' categories return `403` on `PUT` and `DELETE`. Names must be unique per user, case-insensitive, including against the defaults (`409`). All endpoints require a valid JWT. Write tests for listing (including defaults), creating, updating, deleting with expenses reassigned to "Other", and the `403` cases.

## 7. Backend: expenses endpoints
Goal: Implement full CRUD for expenses behind auth.
Description: `GET /expenses` accepts an optional `?month=YYYY-MM` filter. Implement `GET /expenses`, `POST /expenses`, `GET /expenses/:id`, `PUT /expenses/:id`, and `DELETE /expenses/:id`. Each operation must be scoped to the authenticated user — users must never see or modify another user's expenses. Write tests covering create, list, update, and delete, plus an unauthorized-access case.

## 8. Backend: budgets endpoints
Goal: Implement `GET /budgets` and `PUT /budgets` behind auth.
Description: `GET` returns all budget rows for the authenticated user for a given month (passed as a query param, e.g. `?month=2026-10`). `PUT` upserts a budget for a specific category (or the overall budget when `category_id` is omitted). Write tests for fetching an empty month, setting a budget, and updating an existing one.

## 9. Frontend: app shell with routing
Goal: Set up React Router with placeholder pages and a persistent nav bar.
Description: Install React Router and define routes for `/login`, `/register`, `/dashboard`, `/expenses`, `/categories`, and `/budgets`. Each route renders a minimal placeholder component (just a heading). Add a nav bar that links between the authenticated pages and an auth guard that redirects unauthenticated users to `/login`. The guard asks `GET /api/auth/me` (with `credentials: 'include'`) whether a session exists; in tests, mock `fetch`. No other API calls yet.

## 10. Frontend: register and login pages
Goal: Build working auth forms connected to the backend API.
Description: Create `Register` and `Login` form components with controlled inputs for email and password. On submit, call `POST /api/auth/register` or `POST /api/auth/login` with `credentials: 'include'`, so the browser keeps the `session` cookie (never store the token in `localStorage`), and redirect to `/dashboard`. Add a logout button in the nav that calls `POST /api/auth/logout`. Display inline error messages for failed requests (wrong password, email taken, password too short, etc.).

## 11. Frontend: expense list and add-expense form
Goal: Build a page to view and log expenses.
Description: Fetch and display all of the user's expenses from `GET /expenses` in a table (amount, category, date, note). Add a form above or in a modal to create a new expense via `POST /expenses`, with a category dropdown populated from `GET /categories`. Newly added expenses should appear in the list without a full page reload.

## 12. Frontend: category management page
Goal: Build a page to view, create, edit, and delete custom categories.
Description: Fetch and display all categories (defaults + user's custom ones) from `GET /categories`. Provide a small form to create a new custom category (name + color picker), and edit and delete buttons on custom categories. Deleting asks for confirmation and says the category's expenses will move to "Other". Mark default categories visually so the user knows they cannot be edited or deleted. Changes should appear in the list immediately.

## 13. Frontend: budget settings page
Goal: Build a page to set per-category and overall monthly budgets.
Description: Fetch the current month's budgets from `GET /budgets?month=<current>` and display them alongside each category name. Allow the user to type a new amount for any row and save it via `PUT /budgets`. Include a row for the overall monthly budget (no category). Show a success or error indicator after each save.

## 14. Frontend: dashboard with spending pie chart
Goal: Build the dashboard that shows a spending breakdown for the current month.
Description: Use Recharts through the shadcn `Chart` component (see `_docs/design-system.md`) and render a pie chart showing total spending per category for the current month, derived from `GET /expenses?month=<current>`. Below the chart, show a summary table with each category's spent amount vs. its budget (if set) and highlight over-budget categories. All data comes from the existing `GET /expenses` and `GET /budgets` endpoints.

## 15. Frontend: edit and delete expenses
Goal: Let users edit and delete an expense from the expense list.
Description: Follow-up to #11. Add edit and delete actions to each row on `/expenses`, using `PUT` and `DELETE /expenses/:id` from #7, with an in-page delete confirmation.

## 16. Frontend: month picker for dashboard and budgets
Goal: Let users view and set any month, not only the current one.
Description: Follow-up to #13 and #14. Add a previous/next month picker to `/dashboard` and `/budgets` that drives the `?month=` parameter of the existing endpoints.

## 17. Backend: rate-limit login attempts
Goal: Slow down password guessing.
Description: Follow-up to #5. After 5 failed logins for the same email within 15 minutes (or 20 from the same IP, amended by #26), `POST /auth/login` returns `429` with `Retry-After`; in-memory, no new dependency. The login form shows a message on `429`.

## 18. Remove a budget (backend and frontend)
Goal: Let users take a budget away again.
Description: Follow-up to #8 and #13. Add `DELETE /budgets?month=YYYY-MM&category_id=<id>` (overall budget when `category_id` is omitted) and a remove action on `/budgets`.

## 19. Backend: cap expense and budget amounts
Goal: Reject absurd amounts so monthly sums cannot overflow and the frontend never loses precision.
Description: Follow-up to #7 and #8. `POST/PUT /expenses` and `PUT /budgets` return `400` when `amount` is above 1,000,000,000,000 (one trillion tenge, well under 2^53). Document the limit in `_docs/plan.md`. Exactly 1,000,000,000,000 is accepted, one more is rejected; the limit is one shared constant. Out of scope: frontend validation, DB CHECK constraint.

## 20. Backend: login rate limit behind a proxy
Goal: Make the login limiter usable behind a reverse proxy without blocking everyone.
Description: Follow-up to #17. Add an opt-in `TRUSTED_PROXY=true` environment variable; when set, the limiter takes the client IP from the last `X-Forwarded-For` entry, otherwise it keeps using the connection's remote address and ignores the header. No new dependency. Document the variable in `_docs/plan.md`. Only the exact value `true` enables it; a missing, empty, or invalid last entry falls back to the remote address. plan.md has no rate-limit section yet, so add one. Out of scope: proxy allowlist or hop count.

## 21. Backend: apply the schema on startup
Goal: A fresh database works without a manual step.
Description: Follow-up to #3 and #4. `main.go` calls `db.ApplySchema` after connecting, so registering against an empty database no longer fails with a 500. Also log the underlying error when a handler returns `500`, without logging request bodies or secrets. If `ApplySchema` fails, log `apply schema: <err>` and exit non-zero before listening. Every 500 site logs the error, method, and path (no query string, headers, or cookies) through one shared helper. Out of scope: versioned migrations, structured logging.

## 22. Backend: consistent 400 for non-numeric ids
Goal: Make a malformed id behave the same on every route.
Description: Follow-up to #6 and #7. `GET/PUT/DELETE /expenses/{id}` with a non-numeric id returns `400` like the category routes do; a numeric id that does not exist still returns `404`. Update `_docs/plan.md` and the tests. The id is checked before the body is read; `0`, negative, and nonexistent numeric ids and other users' expenses stay `404`; the response body is `invalid expense id`.

## 23. Frontend: validate the amount cap in forms
Goal: Users see a clear message in the form before submitting an amount above the backend limit.
Description: Follow-up to #19. The add/edit expense form and every budget amount input show the inline error `Amount must be at most {formatMoney(1000000000000)}.` above 1,000,000,000,000 and do not call the API; exactly the cap is accepted. One shared frontend validator and constant replace the two `validateAmount` copies, the error clears on edit, and a backend 400 message is shown as is (the budget row must not append the "check your connection" suffix to it). Empty, zero, negative, decimal and non-numeric inputs keep their whole-number messages. Out of scope: thousands separators (#27), reading the limit from the backend (#28).

## 24. Backend: limit trusted proxies by address or hop count
Goal: With TRUSTED_PROXY enabled, the login limiter only trusts `X-Forwarded-For` from listed proxies, so a client cannot choose its own IP.
Description: Follow-up to #20. Add `TRUSTED_PROXIES` (comma-separated IPs/CIDRs, IPv4 and IPv6). With `TRUSTED_PROXY=true` and the list set: a remote address outside the list ignores the header; otherwise walk the joined `X-Forwarded-For` chain from the right, skipping listed proxies, and use the first unlisted entry; malformed entry, empty chain or all-trusted chain falls back to the remote address. Unset list keeps #20 behavior exactly. An invalid list fails startup naming the entry; a list without `TRUSTED_PROXY=true` logs a warning. Document in `_docs/plan.md`. Design (address list first) awaits user confirmation. Out of scope: hop count (#30), other proxy headers (#29).

## 25. Backend: versioned schema migrations
Goal: Schema changes after launch are applied in order and tracked.
Description: Follow-up to #21. Move `schema.sql` to `backend/db/migrations/0001_initial.sql` and add numbered `NNNN_name.sql` migrations, embedded and applied in version order under the existing advisory lock in one all-or-nothing transaction. Applied migrations are recorded in `schema_migrations` (version, name, SHA-256 checksum, applied_at) and never re-run; a fresh database ends in the same state as today's `schema.sql` (6 default categories, once). An existing database with no `schema_migrations` table but a `users` table is adopted by recording version 1 without running it (no data change). A failed migration rolls back and records nothing; a modified applied migration (`modified after it was applied`) or a database newer than the binary (`newer than this binary`) fails startup; gaps, duplicates and bad names fail startup. Concurrent starts apply each migration once. Keep `ApplySchema` signature, add an "adding a migration" note to `_docs/plan.md`. Out of scope: down migrations (#31), status/apply-only command (#32), non-transactional migrations (#33), external tools. Decisions to confirm with the user: checksum failure, newer-DB failure, single transaction, baseline detection.

## 26. Backend: separate login rate limits for email and IP
Goal: Stop shared networks from locking each other out of login, while still throttling password guessing.
Description: Amends #17 and the #17/#20 tests. Two constants: 5 failed logins per email and 20 per client IP, each per 15 minutes; either limit returns `429` with `Retry-After`, even for the correct password, and the response is identical for correct/wrong passwords and known/unknown emails. `Retry-After` is computed per key (when its oldest counted failure leaves the window); if both are blocked, the larger value is used. A success resets only the email counter; `429`, `400` and `500` responses are not counted as failures. The IP is whatever `clientIP` returns (`TRUSTED_PROXY` from #20 unchanged). Update `_docs/plan.md` and the tests (including the #20 proxy tests, now 20/21) so no document says five from the same IP.

## 27. Frontend: accept thousands separators in amount fields
Goal: Users can type `1 500` or `1,500` and it is accepted.
Description: Follow-up to #23. One shared function in `amount.ts` (used by `validateAmount` and `parseAmount`, so both forms) accepts strictly grouped thousands: a first group of 1-3 digits not starting with `0`, then groups of exactly 3, joined by a space, NBSP (U+00A0), narrow NBSP (U+202F) or comma; plain digits work as before. `1 500` and `1,500` are sent as `1500`; `1,00`, `1,5`, `1500,5`, `1500.5`, `1.500`, `0,500`, `12 34`, `1,,000`, trailing/leading separators and `1 500 ₸` are rejected with the whole-number message. Surrounding whitespace from pasting is trimmed. Cap, empty, zero and negative rules apply to the cleaned value. Strict grouping is deliberate so a ru-KZ decimal comma is never read as thousands. Confirmed with the user: strict grouping, space and comma separators, mixed separators allowed, pasted `₸`/`KZT`/`тг` rejected. Out of scope: live grouping while typing (#34), pasted currency marker (#35), tiyn (plan).

## 28. Expose the amount limit from the backend to the frontend
Goal: The backend owns the amount limit and the frontend reads it instead of hard-coding it.
Description: Follow-up to #23. New public `GET /config` (no session, no database, 405 for other methods) returns `{"maxAmount": 1000000000000}` from the same `maxAmount` constant the expense and budget validation use. The frontend requests `/api/config` once per page load (in memory, shared by the expense form and all budget rows) and removes `MAX_AMOUNT` from `amount.ts`. While loading, and if the load fails (network error, non-2xx, malformed body), the forms are not blocked and do not check the cap client-side, so the backend 400 message is shown as is. Once loaded the cap message uses the loaded value; other amount rules (#23, #27) are unchanged. Go and frontend tests on both sides (including a test that the endpoint value equals the enforced limit) and `GET /config` documented in `_docs/plan.md`. Out of scope: changing the limit (#19), retrying a failed load (#36), other limits (#37). Decided with the user: new endpoint, loaded once, fall back to the backend 400. To confirm: forms are not blocked while loading.

## 29. Backend: support other proxy headers for the client IP
Goal: Let operators use `X-Real-IP`, `Forwarded` or similar instead of `X-Forwarded-For`.
Description: Follow-up to #24. Design to be agreed with the user first.

## 30. Backend: trusted proxy hop count as an alternative to the address list
Goal: With TRUSTED_PROXY enabled, an operator can set the number of proxies in front of the server instead of listing their addresses.
Description: Follow-up to #24. Add `TRUSTED_PROXY_HOPS=N` (integer 1-64, spaces trimmed): the client IP is the Nth entry from the right of the joined `X-Forwarded-For` chain (N=1 equals the #20 default). A chain shorter than N, or a malformed Nth entry, falls back to the remote address. Zero, negative, non-numeric or above 64 fails startup with `invalid TRUSTED_PROXY_HOPS "<value>"`; setting it together with `TRUSTED_PROXIES` also fails startup; setting it without `TRUSTED_PROXY=true` logs a warning and is ignored. The remote address is not checked in this mode. Go tests for `clientIP` and startup validation, and `_docs/plan.md` documents it. Out of scope: combining with the address list (#38), logging fallbacks (#39), other proxy headers (#29). Decisions made while grooming, see the issue Constraints.

## 31. Backend: down (rollback) migrations
Goal: An operator can undo the latest applied migration with `budgetcontrol migrate down`, never as a side effect of starting the server.
Description: Follow-up to #25. Adds the `migrate status|up|down` subcommand dispatch of the existing binary (reads only `DATABASE_URL`, exit codes 0/1/2; `status` and `up` are completed in #32). Down files are embedded `NNNN_name.down.sql` next to the up file; a down file without an up file fails loading; `0001` has none. `down` reverts only the single latest applied migration in one transaction under the advisory lock and deletes its `schema_migrations` row; a missing down file, a checksum mismatch, a newer-than-binary database or a hole is an error that changes nothing; nothing applied prints `nothing to revert` and does not create the table or adopt a legacy database. The server never runs down. Go tests, and `_docs/plan.md` documents it (including that starting the server after `down` re-applies). Out of scope: several steps or a target version (#40), confirmation/production guard (#41), status and `up` (#32), non-transactional (#33). Decisions made while grooming, see the issue Constraints.

## 32. Backend: migration status and apply-only command
Goal: An operator can see applied and pending migrations with `migrate status` and apply them with `migrate up`, without starting the server.
Description: Follow-up to #25. Completes the `migrate` subcommand of #31 (only `DATABASE_URL`, exit codes 0/1/2, results on stdout, errors on stderr). `status` is read-only (no table creation, no legacy adoption, no lock) and prints one tab-separated line per migration (`NNNN`, description, `applied|pending`, checksum state `ok|mismatch|-|hole`; a version the binary lacks is `applied newer`) plus a `<A> applied, <P> pending` summary; it exits 1 (after printing the listing) on mismatch, newer-than-binary or a hole, and 0 when migrations are merely pending; a legacy database is reported all pending with a stderr note. `up` reuses `ApplyMigrations` (same lock, one transaction, adoption), prints `applied <NNNN_name>` per migration (`adopted 0001_initial` for a legacy database) after the commit, or `nothing to apply`; a failure applies and prints nothing. Go tests, and `_docs/plan.md` documents both verbs. Out of scope: JSON output (#42), status check mode for CI (#43), dry run / target version (#44), applied time and down availability in status (#45), non-transactional (#33). Decisions made while grooming, see the issue Constraints.

## 33. Backend: non-transactional migrations
Goal: A migration that cannot run in a transaction (e.g. `CREATE INDEX CONCURRENTLY`) can opt out with a marker and be applied safely.
Description: Follow-up to #25. A migration (up or down) is non-transactional when its first line is `-- migrate:no-transaction` (name and checksum rules unchanged, `0001` cannot use it, exactly one statement per file). Pending migrations still run in version order; consecutive transactional ones share one transaction and a non-transactional one commits the open group first, so atomicity is per group. A non-transactional migration writes a row to `schema_migrations_in_progress`, runs its statement outside a transaction, then records it in `schema_migrations` and removes the row in its own transaction. When the migration set contains a non-transactional file the advisory lock is a session-level lock, taken before any transaction (waiters poll, so `CREATE INDEX CONCURRENTLY` cannot deadlock with them) and held for the whole run. Any failure leaves the in-progress row; startup, `migrate up` and `migrate down` then fail with `did not finish` and the recovery SQL, and `migrate status` shows the migration as `pending` / `interrupted` (exit 1). A non-transactional down file is marked the same way. Go tests against Postgres (`CREATE INDEX CONCURRENTLY`, failing unique index, session lock, concurrent callers) and `_docs/plan.md` documents it. Out of scope: `migrate resolve` (#46), multi-statement files (#47), showing non-transactional in status (#48). Decisions made while grooming, see the issue Constraints.

## 34. Frontend: live thousands grouping while typing in amount fields
Goal: While the user types in an amount field, the digits are shown grouped (e.g. `1 500`) without moving the cursor unexpectedly.
Description: Follow-up to #27. Design (grouping character, cursor and paste behavior) to be agreed with the user first; only if the user wants it.

## 35. Frontend: accept a currency marker when pasting an amount
Goal: A value pasted from the app's own display, such as `1 500 ₸`, is accepted in amount fields.
Description: Follow-up to #27, which rejects `₸`, `KZT` and `тг`. Must define which markers are stripped and where. Only if the user wants it.

## 36. Frontend: retry loading the amount limit after a failure
Goal: A failed `GET /config` load is retried later instead of staying unknown until reload.
Description: Follow-up to #28. Design (retry on next form open, timed, or manual) to be agreed with the user first.

## 37. Backend: expose other validation limits through GET /config
Goal: Other limits the frontend duplicates (e.g. minimum password length) are read from `GET /config`.
Description: Follow-up to #28. Which limits to expose to be agreed with the user first; only if the user wants it.

## 38. Backend: combine trusted proxy hop count with the address list
Goal: An operator can use `TRUSTED_PROXY_HOPS` and `TRUSTED_PROXIES` together.
Description: Follow-up to #30, where setting both is a startup error. Design to be agreed with the user first; standard library only.

## 39. Backend: log client IP fallbacks behind a trusted proxy
Goal: An operator can see when the proxy setup does not match the configured hop count.
Description: Follow-up to #30. Log (rate limited) when the client IP falls back to the remote address. Design to be agreed with the user first; standard library only.

## 40. Backend: migrate down for several steps or to a target version
Goal: An operator can revert more than one migration in a single run.
Description: Follow-up to #31, which reverts only the latest. Design to be agreed with the user first; standard library only.

## 41. Backend: safety guard for destructive migrate down
Goal: `migrate down` cannot be run against production by accident.
Description: Follow-up to #31, whose `down` runs without confirmation. Decide on a confirmation flag or an `APP_ENV=production` refusal. Design to be agreed with the user first; standard library only.

## 42. Backend: machine-readable output for migrate status
Goal: Scripts can read migration status without parsing text.
Description: Follow-up to #31/#32. Optional `--json` output. Design to be agreed with the user first; standard library only.
