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
- `TRUSTED_PROXY` (default off; only the exact value `true` enables it): the client IP for the limit is normally the connection's remote address and `X-Forwarded-For` is ignored. Enable it only behind a reverse proxy that appends the real client IP to `X-Forwarded-For`, otherwise clients can forge the header. When enabled, the last entry of the last `X-Forwarded-For` line is used; a missing or invalid value falls back to the remote address
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
- Amounts must be positive integers of at most 1,000,000,000,000 (one trillion tenge), for expenses and budgets alike.
- The frontend formats with `Intl.NumberFormat('ru-KZ', { style: 'currency', currency: 'KZT' })`.

## Database Tables (Planned)
- `users` — id, email, password_hash, created_at
- `categories` — id, user_id (null = default), name, color
- `expenses` — id, user_id, category_id, amount (BIGINT, whole tenge), date, note, created_at
- `budgets` — id, user_id, category_id (null = overall), amount (BIGINT, whole tenge), month (DATE, always the 1st of the month, enforced by a CHECK constraint)

## Next Steps
1. Set up Neon database and schema
2. Build Go REST API with auth
3. Build React frontend
