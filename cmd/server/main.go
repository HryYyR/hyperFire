package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"agentDemo/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(server.Config{
		TCPAddr: ":7000",
		UDPAddr: ":7001",
		TickHz:  60,
	}, log.Default())

	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
