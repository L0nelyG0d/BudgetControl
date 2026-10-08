- `go mod download` - install dependencies
- `go test ./...` - the whole suite
- `go test ./path/to/pkg` - one package
- `go test -run TestName ./...` - one test by name

Rules

- Dependencies are tracked in `go.mod` (backend) and `frontend/package.json` (frontend). Do not add one without asking
- Pre-approved frontend packages: those listed under "Approved packages" in `_docs/design-system.md`
- Pre-approved Go modules: `github.com/jackc/pgx/v5`, `golang.org/x/crypto` (bcrypt), `github.com/golang-jwt/jwt/v5`. Everything else comes from the standard library: routing uses `net/http` patterns (no chi or other router)

Documents

- `_docs/plan.md` - what the app does: features, endpoints, tables, money rules
- `_docs/tasks.md` - the backlog, one task per GitHub issue
- `_docs/process.md` - how work is organized
- Before writing tests, read `_docs/testing-guidelines.md`
- For anything touching the UI, read `_docs/design-system.md`
