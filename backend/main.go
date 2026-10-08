package main

import (
	"context"
	"log"
	"net/http"
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

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set; set it to the PostgreSQL connection string")
	}
	deps := api.Deps{
		JWTSecret:  []byte(os.Getenv("JWT_SECRET")),
		Production: os.Getenv("APP_ENV") == "production",
	}
	if err := deps.Validate(); err != nil {
		log.Fatal(err)
	}
	pool, err := db.Connect(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	deps.DB = pool

	a := addr()
	log.Printf("listening on %s", a)
	log.Fatal(http.ListenAndServe(a, newMux(deps)))
}
