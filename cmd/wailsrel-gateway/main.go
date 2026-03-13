package main

import (
	"log"
	"net/http"

	"github.com/you/wailsrel/internal/gateway"
)

func main() {
	cfg, err := gateway.LoadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	server, err := gateway.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("wailsrel gateway listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server); err != nil {
		log.Fatal(err)
	}
}
