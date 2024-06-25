package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zoujiaqing/qrpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("s", "localhost", "server")
	port := flag.Uint("p", 4444, "port")
	flag.Parse()

	client := qrpc.NewClient(*addr, *port)

	client.OnRequest(func(data []byte) []byte {
		return append([]byte("From Client Respond "), data...)
	})

	if err := client.Connect(); err != nil {
		return err
	}

	data, err := client.Request([]byte("Hello"))
	if err != nil {
		log.Printf("Request error: %v", err)
	}
	log.Printf("Respond: %s", string(data))

	for {
		// print ping value
		log.Printf("Ping: %d", client.GetPingValue())
		time.Sleep(1 * time.Second)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs

	client.Disconnect()

	log.Println("Close rpc client")

	return nil
}
