package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/UnitVectorY-Labs/invdex/internal/config"
	"github.com/UnitVectorY-Labs/invdex/internal/database"
	"github.com/UnitVectorY-Labs/invdex/internal/handlers"
	"github.com/UnitVectorY-Labs/invdex/internal/llm"
	"github.com/UnitVectorY-Labs/invdex/internal/storage"
)

// Version is the application version, injected at build time via ldflags
var Version = "dev"

func main() {
	if Version == "dev" || Version == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				Version = bi.Main.Version
			}
		}
	}

	fmt.Printf("invdex %s\n", Version)
	fmt.Println("A personal inventory system powered by LLMs.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Connect to database
	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Initialize storage
	store, err := storage.New(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}

	// Initialize LLM client
	llmClient := llm.New(cfg)

	// Initialize handlers
	h, err := handlers.New(db, store, llmClient)
	if err != nil {
		log.Fatalf("failed to initialize handlers: %v", err)
	}

	// Set up routes
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Serve static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"` + Version + `"}`))
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
		cancel()
	}()

	log.Printf("starting server on port %d", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
