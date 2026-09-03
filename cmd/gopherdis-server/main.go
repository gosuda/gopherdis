package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/gosuda/gopherdis/server"
)

func main() {
	debug.SetGCPercent(200)
	port := flag.Int("port", 6379, "TCP port to listen on")
	host := flag.String("host", "0.0.0.0", "Host interface to bind")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	srv := server.NewServer()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down Gopherdis...")
		_ = srv.Close()
		os.Exit(0)
	}()

	log.Printf("Gopherdis server starting on %s ...\n", addr)
	if err := srv.Listen(addr); err != nil {
		log.Fatalf("Gopherdis failed: %v", err)
	}
}
