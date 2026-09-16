package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/gosuda/gopherdis/aof"
	"github.com/gosuda/gopherdis/server"
)

func main() {
	debug.SetGCPercent(200)
	port := flag.Int("port", 6379, "TCP port to listen on")
	host := flag.String("host", "0.0.0.0", "Host interface to bind")
	appendOnly := flag.Bool("appendonly", false, "Persist writes to an append-only file")
	appendFile := flag.String("appendfilename", "appendonly.aof", "Append-only file path")
	appendFsync := flag.String("appendfsync", "everysec", "AOF fsync policy: always, everysec or no")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	srv := server.NewServer()

	if *appendOnly {
		var policy aof.FsyncPolicy
		switch *appendFsync {
		case "always":
			policy = aof.FsyncAlways
		case "everysec":
			policy = aof.FsyncEverySec
		case "no":
			policy = aof.FsyncNo
		default:
			log.Fatalf("invalid -appendfsync %q: want always, everysec or no", *appendFsync)
		}
		// Replays the existing file before Listen, so the server never serves an
		// empty keyspace while loading.
		if err := srv.EnableAOF(*appendFile, policy); err != nil {
			log.Fatalf("Gopherdis failed to start: %v", err)
		}
		log.Printf("AOF enabled: %s (fsync=%s), %d keys loaded\n", *appendFile, *appendFsync, srv.DB.Len())
	}

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
