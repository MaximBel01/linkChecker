package main

import (
	"context"
	"link-checker/internal/api"
	"link-checker/pkg/checker"
	"link-checker/pkg/pdf"
	"link-checker/pkg/storage"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	port    = ":8080"
	dataDir = "data"
	timeout = 10 * time.Second
)

func main() {
	log.Println("Starting Link Checker Server...")

	store, err := storage.NewStorage(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Println("Storage initialized successfully")

	pendingBatches := store.ListPendingBatches()
	if len(pendingBatches) > 0 {
		log.Printf("Found %d pending batches to resume\n", len(pendingBatches))
		resumeProcessing(store, pendingBatches)
	}

	linkChecker := checker.NewLinkChecker(timeout)
	pdfGen := pdf.NewGenerator()

	handler := api.NewHandler(linkChecker, store, pdfGen)

	http.HandleFunc("/health", handler.HandleHealth)
	http.HandleFunc("/check", handler.HandleCheckLinks)
	http.HandleFunc("/report", handler.HandleGetReport)
	http.HandleFunc("/status", handler.HandleGetStatus)

	server := &http.Server{
		Addr:         port,
		Handler:      http.DefaultServeMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server started on http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sig := <-shutdownChan
	log.Printf("\nReceived signal: %v", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Shutting down server gracefully...")

	store.WaitForCompletion(ctx)

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

func resumeProcessing(store *storage.Storage, pendingBatches []*storage.LinkBatch) {
	linkChecker := checker.NewLinkChecker(timeout)

	for _, batch := range pendingBatches {
		go func(b *storage.LinkBatch) {
			log.Printf("Resuming batch %d with %d links\n", b.BatchID, len(b.URLs))

			store.UpdateBatch(b.BatchID, []any{}, "processing")

			results := linkChecker.CheckLinks(b.URLs)

			var resultInterfaces []any
			for _, result := range results {
				resultMap := map[string]any{
					"url":        result.URL,
					"status":     result.Status,
					"available":  result.Available,
					"checked_at": result.CheckedAt,
				}
				if result.Error != "" {
					resultMap["error"] = result.Error
				}
				resultInterfaces = append(resultInterfaces, resultMap)
			}

			store.UpdateBatch(b.BatchID, resultInterfaces, "completed")
			log.Printf("Batch %d completed\n", b.BatchID)
		}(batch)
	}
}
