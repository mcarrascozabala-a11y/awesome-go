// Command muller-app serves the "Saisie Ligne Muller" shift-entry app:
// a JSON API backed by SQLite and an installable web app (PWA) for the
// shop-floor tablets.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	// On stop (Ctrl-C, docker stop, systemctl stop), finish pending requests
	// and close the database so its file is complete and safe to copy.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("Ligne Muller listening on %s (db %s)", *addr, *dbPath)
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Print("stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
