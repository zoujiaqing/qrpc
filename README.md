# QRPC

A simple rpc framework that works over [QUIC](https://en.wikipedia.org/wiki/QUIC) written in Golang.

## Client sample code

```go
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

	if err := client.Connect(); err != nil {
		return err
	}

	client.OnRequest(func(data []byte) []byte {
		return append([]byte("From Client Respond "), data...)
	})

	var i uint64 = 1
	for {
		data, err := client.Request([]byte("Hello"))
		if err != nil {
			log.Printf("Request error: %v", err)
		}
		log.Printf("Respond(%d): %s", i, string(data))
		time.Sleep(1 * time.Second)
		i++
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs

	client.Disconnect()

	log.Println("Close rpc client")

	return nil
}

```

## Server sample code

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zoujiaqing/qrpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("unhandled application error: %s\n", err.Error())
		os.Exit(1)
	}
}

func run() error {

	port := flag.Uint("p", 4444, "port")
	flag.Parse()

	server, err := qrpc.NewServer(*port)
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Accept(ctx)

	log.Println("server started")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs

	log.Println("shutting down server")

	return nil
}

```
