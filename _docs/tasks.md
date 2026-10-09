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
Description: Follow-up to #19. The expense form and the budget amount input show an inline error above 1,000,000,000,000 and do not call the API.

## 24. Backend: limit trusted proxies by address or hop count
Goal: With TRUSTED_PROXY enabled, a client cannot spoof its IP with its own X-Forwarded-For.
Description: Follow-up to #20. Let the operator say which proxies are trusted (address list or hop count); document it in `_docs/plan.md`.

## 25. Backend: versioned schema migrations
Goal: Schema changes after launch are applied in order and tracked.
Description: Follow-up to #21. Record applied migrations in a table and never re-run them; a fresh database ends in the same state as today's `schema.sql`.

## 26. Backend: separate login rate limits for email and IP
Goal: Stop shared networks from locking each other out of login, while still throttling password guessing.
Description: Amends #17 and the #17/#20 tests. Two constants: 5 failed logins per email and 20 per client IP, each per 15 minutes; either limit returns `429` with `Retry-After`, even for the correct password, and the response is identical for correct/wrong passwords and known/unknown emails. `Retry-After` is computed per key (when its oldest counted failure leaves the window); if both are blocked, the larger value is used. A success resets only the email counter; `429`, `400` and `500` responses are not counted as failures. The IP is whatever `clientIP` returns (`TRUSTED_PROXY` from #20 unchanged). Update `_docs/plan.md` and the tests (including the #20 proxy tests, now 20/21) so no document says five from the same IP.
