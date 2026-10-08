# Process

## Frontend design
- General  rontend design can be accessed at `_docs/design-system.md` file. If there is a task that requires changes in the frontend read the file first to follow the general guideline. If you have any questions regarding the design consulate with the user before adding changes yourself. Give several options to choose from and demonstrate how they would look.
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

## Commits
- Work directly on `main`. No branch or pull request per issue
- Commit regularly: small commits that each leave the tests passing
- Commit messages are short and in the imperative ("Add register endpoint"). Reference the issue (`Refs #4`), and use `Closes #4` on the last commit of the issue
- Push only when the user asks

## Secrets
- Never commit tokens, passwords, or connection strings. Keep `JWT_SECRET`, `DATABASE_URL`, and `TEST_DATABASE_URL` in environment variables or a git-ignored `.env`
- Never paste a secret into a doc, an issue, or a commit message

- PM - grooms a task before anyone implements it, follows _docs/team/pm.md

