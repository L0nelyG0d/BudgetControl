package main

import (
	"context"
	"errors"
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

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set; set it to the PostgreSQL connection string")
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
