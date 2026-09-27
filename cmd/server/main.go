package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/example/botshelf/internal/catalog"
	"github.com/example/botshelf/internal/httpapi"
	"github.com/example/botshelf/internal/maxauth"
)

// web contains the responsive web interface. It can be served locally during
// development and later attached to a MAX bot as a mini-application.
//
//go:embed web/*
var webFS embed.FS

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	store, err := catalog.OpenPostgres(startupCtx, requiredEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("open PostgreSQL store: %v", err)
	}
	defer store.Close()

	moderatorIDs := parseModeratorIDs(requiredEnv("MODERATOR_MAX_USER_IDS"))
	auth := maxauth.New(store, requiredEnv("MAX_BOT_TOKEN"), moderatorIDs)
	server := httpapi.NewServer(store, auth, store)
	mux := http.NewServeMux()
	server.Register(mux)

	staticFiles, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("load embedded web files: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFiles)))

	httpServer := &http.Server{
		Addr:              envOr("HTTP_ADDR", ":8080"),
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("BotShelf is listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s must be configured", name)
	}
	return value
}

func parseModeratorIDs(raw string) []int64 {
	parts := strings.Split(raw, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			log.Fatalf("MODERATOR_MAX_USER_IDS contains an invalid MAX user ID")
		}
		result = append(result, id)
	}
	if len(result) == 0 {
		log.Fatal("MODERATOR_MAX_USER_IDS must contain at least one ID")
	}
	return result
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; script-src 'self' https://st.max.ru; style-src 'self'; img-src 'self' data: https:; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
