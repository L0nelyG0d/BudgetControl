# Personal Expense Tracker — Project Scope

## Overview
A fullstack web application for tracking personal expenses with category breakdowns, budgets, and visualizations.

## Tech Stack
- **Frontend:** React
- **Backend:** Go (REST API)
- **Database:** Neon (PostgreSQL)
- **Auth:** Email/password with JWT

## Features

### Authentication
- Register with email + password
- Login with email + password
- JWT-based session management

### Expense Logging
- Log an expense with:
  - Amount
  - Category
  - Date
  - Optional note

### Categories
- Default categories: Food, Transport, Housing, Entertainment, Health, etc.
- Users can create custom categories on top of defaults

### Budgets
- Per-category monthly budget (e.g., Food: $500/month)
- Overall monthly budget
- Budget vs. actual spending tracking

### Charts & Visualization
- Pie chart: spending breakdown by category for the current month

## Project Structure

### Backend (Go)
- JWT authentication middleware
- REST endpoints:
  - `POST /auth/register`
  - `POST /auth/login`
  - `GET/POST /expenses`
  - `GET/PUT/DELETE /expenses/:id`
  - `GET/POST /categories`
  - `GET/PUT /budgets`
- Neon/PostgreSQL for data storage

### Frontend (React)
- Auth pages: Login / Register
- Dashboard: pie chart + monthly summary
- Expense list + add expense form
- Budget settings page
- Category management

## Database Tables (Planned)
- `users` — id, email, password_hash, created_at
- `categories` — id, user_id (null = default), name, color
- `expenses` — id, user_id, category_id, amount, date, note, created_at
- `budgets` — id, user_id, category_id (null = overall), amount, month

## Next Steps
1. Set up Neon database and schema
2. Build Go REST API with auth
3. Build React frontend
