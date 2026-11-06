// Command muller-app serves the "Saisie Ligne Muller" shift-entry app:
// a JSON API backed by SQLite and an installable web app (PWA) for the
// shop-floor tablets.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", envOr("MULLER_ADDR", ":"+envOr("PORT", "8080")), "listen address")
	dbPath := flag.String("db", envOr("MULLER_DB", "muller.db"), "SQLite database file")
	flag.Parse()

	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	static, _ := fs.Sub(webFS, "web")
	srv := &Server{Store: store, Static: static, Password: os.Getenv("MULLER_PASSWORD")}
	if srv.Password == "" {
		log.Print("warning: MULLER_PASSWORD is not set, the app is open to anyone who can reach it")
	}
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("Ligne Muller listening on %s (db %s)", *addr, *dbPath)
	log.Fatal(httpSrv.ListenAndServe())
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
