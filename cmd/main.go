// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "ukrainian_call_center_scam_goev/internal/billing/handlers/http"
	memory2 "ukrainian_call_center_scam_goev/internal/billing/repo/memory"
	"ukrainian_call_center_scam_goev/internal/billing/service"
	"ukrainian_call_center_scam_goev/web"
)

func main() {
	addr := env("ADDR", ":8080")

	tariffRepo := memory2.NewTariffMemoryRepo()
	subscriberRepo := memory2.NewSubscriberMemoryRepo()

	// Service
	svc := billing.NewWithConfig(tariffRepo, subscriberRepo, time.UTC, 2)

	// HTTP handlers
	h := httpapi.NewHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("/", http.FileServer(http.FS(web.UI)))

	// optional: middleware logging
	handler := withRequestLog(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// Важно: ReadTimeout не ставим маленьким, иначе стриминг больших файлов может обрываться.
		IdleTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// graceful shutdown
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutdown: start")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	} else {
		log.Println("shutdown: done")
	}
}

func env(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
