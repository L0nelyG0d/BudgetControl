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

// runMigrate implements `budgetcontrol migrate <verb>` and returns the exit
// code: 0 success, 1 failure, 2 usage error. args are the arguments after
// "migrate". Arguments are validated before the database is touched. revert
// connects to the database at the given URL and reverts the latest
// migration, returning its name ("" when nothing is applied).
func runMigrate(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
	revert func(ctx context.Context, dbURL string) (string, error)) int {
	if len(args) != 1 || (args[0] != "status" && args[0] != "up" && args[0] != "down") {
		fmt.Fprintln(stderr, migrateUsage)
		return 2
	}
	switch args[0] {
	case "down":
		dbURL := getenv("DATABASE_URL")
		if dbURL == "" {
			fmt.Fprintln(stderr, errNoDatabaseURL)
			return 1
		}
		name, err := revert(ctx, dbURL)
		if err != nil {
			fmt.Fprintf(stderr, "migrate down: %v\n", err)
			return 1
		}
		if name == "" {
			fmt.Fprintln(stdout, "nothing to revert")
		} else {
			fmt.Fprintf(stdout, "reverted %s\n", name)
		}
		return 0
	default: // status, up: delivered by #32
		fmt.Fprintf(stderr, "migrate %s: not implemented\n", args[0])
		return 2
	}
}

// revertLatest is the production revert used by `migrate down`.
func revertLatest(ctx context.Context, dbURL string) (string, error) {
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		return "", err
	}
	defer pool.Close()
	return db.RevertLatest(ctx, pool)
}

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] != "migrate" {
			fmt.Fprintln(os.Stderr, "usage: budgetcontrol [migrate status|up|down]")
			os.Exit(2)
		}
		os.Exit(runMigrate(context.Background(), os.Args[2:], os.Stdout, os.Stderr, os.Getenv, revertLatest))
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
