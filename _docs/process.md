# Process

## Tasks
- Tasks are GitHub issues in `L0nelyG0d/BudgetControl`, one at a time
- `_docs/tasks.md` is the backlog. Issue numbers match task numbers (task 4 = issue #4)
- Work in order, because later tasks depend on earlier ones. Do not start the next issue until the current one is closed
- If `tasks.md` and an issue disagree, `tasks.md` is the source of truth. Tell the user, and update the issue with `gh issue edit`

## Acceptance criteria
- Read the issue's Goal and Description before starting
- Read them again before closing, and check each point against the actual result
- Close an issue only when its tests pass (`go test ./...` for backend, `npm test` for frontend) and `go test -race ./...` is clean for backend work
- If something in the issue is unclear or conflicts with `_docs/plan.md`, ask before building

## Branches and commits
- One branch per issue, named `<number>-<short-slug>` (e.g. `4-register-endpoint`). Do not commit to `main` directly
- Commit regularly: small commits that each leave the tests passing
- Commit messages are short and in the imperative ("Add register endpoint"). Reference the issue (`Refs #4`)
- Open a pull request per issue with `Closes #4` in the description, so merging closes the issue

## Secrets
- Never commit tokens, passwords, or connection strings. Keep `JWT_SECRET`, `DATABASE_URL`, and `TEST_DATABASE_URL` in environment variables or a git-ignored `.env`
- Never paste a secret into a doc, an issue, or a commit message
