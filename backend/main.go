package main

import (
	"context"
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

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set; set it to the PostgreSQL connection string")
	}
	proxies, err := trustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		log.Fatal(err)
	}
	if len(proxies) > 0 && !trustedProxy() {
		log.Print("warning: TRUSTED_PROXIES is set but TRUSTED_PROXY is not true; the list is ignored")
	}
	deps := api.Deps{
		JWTSecret:      []byte(os.Getenv("JWT_SECRET")),
		Production:     os.Getenv("APP_ENV") == "production",
		TrustedProxy:   trustedProxy(),
		TrustedProxies: proxies,
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
