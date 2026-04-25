package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"mcp_server_scraper_googlemaps/internal/httpapi"
	"mcp_server_scraper_googlemaps/internal/mcp"
	"mcp_server_scraper_googlemaps/internal/scraper"
)

func main() {
	httpAddr := flag.String("http", "", "run HTTP server on this address instead of MCP stdio, e.g. :3000")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := scraper.New(logger)
	if *httpAddr != "" {
		server := httpapi.New(s, logger)
		if err := server.ListenAndServe(ctx, *httpAddr); err != nil && err != http.ErrServerClosed {
			logger.Fatal(err)
		}
		return
	}

	server := mcp.New(os.Stdin, os.Stdout, s, logger)
	if err := server.Serve(ctx); err != nil && err != context.Canceled {
		logger.Fatal(err)
	}
}
