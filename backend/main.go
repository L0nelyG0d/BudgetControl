package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"

	"budgetcontrol/api"
	"budgetcontrol/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newMux(deps api.Deps) *http.ServeMux {
	return api.NewRouter(deps)
}

func addr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

// trustedProxy reports whether TRUSTED_PROXY is exactly "true".
func trustedProxy() bool {
	return os.Getenv("TRUSTED_PROXY") == "true"
}

// trustedProxies parses TRUSTED_PROXIES; see api.ParseTrustedProxies.
func trustedProxies(value string) ([]netip.Prefix, error) {
	return api.ParseTrustedProxies(value)
}

// proxyConfig parses TRUSTED_PROXIES and TRUSTED_PROXY_HOPS. It returns the
// list, the hop count (0 if unset), warnings to log, or an error that stops
// the server. Setting both is an error even when trusted is false.
func proxyConfig(proxiesValue, hopsValue string, trusted bool) (proxies []netip.Prefix, hops int, warnings []string, err error) {
	proxies, err = trustedProxies(proxiesValue)
	if err != nil {
		return nil, 0, nil, err
	}
	hops, err = api.ParseTrustedProxyHops(hopsValue)
	if err != nil {
		return nil, 0, nil, err
	}
	if hops > 0 && len(proxies) > 0 {
		return nil, 0, nil, errors.New("only one of TRUSTED_PROXIES and TRUSTED_PROXY_HOPS may be set")
	}
	if len(proxies) > 0 && !trusted {
		warnings = append(warnings, "warning: TRUSTED_PROXIES is set but TRUSTED_PROXY is not true; the list is ignored")
	}
	if hops > 0 && !trusted {
		warnings = append(warnings, "warning: TRUSTED_PROXY_HOPS is set but TRUSTED_PROXY is not true; the hop count is ignored")
	}
	return proxies, hops, warnings, nil
}

const migrateUsage = "usage: budgetcontrol migrate status|up|down"

const errNoDatabaseURL = "DATABASE_URL is not set; set it to the PostgreSQL connection string"

// migrateOps are the database-backed operations behind `migrate`; each
// connects to the given URL. They are injected so runMigrate is testable
// without a database.
type migrateOps struct {
	// revert reverts the latest migration, returning its name ("" if none).
	revert func(ctx context.Context, dbURL string) (string, error)
	// status reads the migration state without writing anything.
	status func(ctx context.Context, dbURL string) (db.StatusReport, error)
	// up applies the pending migrations like server startup.
	up func(ctx context.Context, dbURL string) (db.ApplyResult, error)
}

// runMigrate implements `budgetcontrol migrate <verb>` and returns the exit
// code: 0 success, 1 failure, 2 usage error. args are the arguments after
// "migrate". Arguments are validated before the database is touched.
func runMigrate(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string, ops migrateOps) int {
	if len(args) != 1 || (args[0] != "status" && args[0] != "up" && args[0] != "down") {
		fmt.Fprintln(stderr, migrateUsage)
		return 2
	}
	verb := args[0]
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(stderr, errNoDatabaseURL)
		return 1
	}
	switch verb {
	case "down":
		name, err := ops.revert(ctx, dbURL)
		if err != nil {
			fmt.Fprintf(stderr, "migrate down: %v\n", err)
			return 1
		}
		if name == "" {
			fmt.Fprintln(stdout, "nothing to revert")
		} else {
			fmt.Fprintf(stdout, "reverted %s\n", name)
		}
	case "up":
		res, err := ops.up(ctx, dbURL)
		// After a failure res lists only the migrations that were committed
		// before it (non-transactional migrations commit separately).
		if res.Adopted {
			fmt.Fprintln(stdout, "adopted 0001_initial")
		}
		for _, name := range res.Applied {
			fmt.Fprintf(stdout, "applied %s\n", name)
		}
		if err != nil {
			fmt.Fprintf(stderr, "migrate up: %v\n", err)
			return 1
		}
		if !res.Adopted && len(res.Applied) == 0 {
			fmt.Fprintln(stdout, "nothing to apply")
		}
	case "status":
		rep, err := ops.status(ctx, dbURL)
		if err != nil {
			fmt.Fprintf(stderr, "migrate status: %v\n", err)
			return 1
		}
		for _, l := range rep.Lines {
			fmt.Fprintln(stdout, l)
		}
		fmt.Fprintln(stdout, rep.Summary())
		if rep.Legacy {
			fmt.Fprintln(stderr, "note: legacy database; migrate up will record 0001_initial as applied without running it")
		}
		for _, p := range rep.Problems {
			fmt.Fprintf(stderr, "migrate status: %v\n", p)
		}
		if len(rep.Problems) > 0 {
			return 1
		}
	}
	return 0
}

// withPool connects to dbURL, runs f and closes the pool.
func withPool[T any](ctx context.Context, dbURL string, f func(context.Context, *pgxpool.Pool) (T, error)) (T, error) {
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		var zero T
		return zero, err
	}
	defer pool.Close()
	return f(ctx, pool)
}

// productionOps are the operations main passes to runMigrate.
var productionOps = migrateOps{
	revert: func(ctx context.Context, dbURL string) (string, error) {
		return withPool(ctx, dbURL, db.RevertLatest)
	},
	status: func(ctx context.Context, dbURL string) (db.StatusReport, error) {
		return withPool(ctx, dbURL, db.Status)
	},
	up: func(ctx context.Context, dbURL string) (db.ApplyResult, error) {
		return withPool(ctx, dbURL, db.ApplyEmbedded)
	},
}

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] != "migrate" {
			fmt.Fprintln(os.Stderr, "usage: budgetcontrol [migrate status|up|down]")
			os.Exit(2)
		}
		os.Exit(runMigrate(context.Background(), os.Args[2:], os.Stdout, os.Stderr, os.Getenv, productionOps))
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal(errNoDatabaseURL)
	}
	proxies, hops, warnings, err := proxyConfig(os.Getenv("TRUSTED_PROXIES"), os.Getenv("TRUSTED_PROXY_HOPS"), trustedProxy())
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range warnings {
		log.Print(w)
	}
	deps := api.Deps{
		JWTSecret:        []byte(os.Getenv("JWT_SECRET")),
		Production:       os.Getenv("APP_ENV") == "production",
		TrustedProxy:     trustedProxy(),
		TrustedProxies:   proxies,
		TrustedProxyHops: hops,
	}
	if err := deps.Validate(); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := db.ApplySchema(ctx, pool); err != nil {
		log.Fatalf("apply schema: %v", err)
	}

	deps.DB = pool

	a := addr()
	log.Printf("listening on %s", a)
	log.Fatal(http.ListenAndServe(a, newMux(deps)))
}
