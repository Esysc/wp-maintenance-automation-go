package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/apiserver"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/webserver"
)

func main() {
	log.Println("Starting combined server...")

	go apiserver.Run()

	go webserver.Run()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
}
