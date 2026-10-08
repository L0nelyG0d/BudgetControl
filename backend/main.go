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
	pool, err := db.Connect(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	a := addr()
	log.Printf("listening on %s", a)
	log.Fatal(http.ListenAndServe(a, newMux(api.Deps{DB: pool})))
}
