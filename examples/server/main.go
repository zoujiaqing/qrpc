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

	onReqeustHandle := func(rpcConn *qrpc.RpcConnection, data []byte) []byte {
		return append([]byte("Respond "), data...)
	}

	server.OnConnection(func(rpcConn *qrpc.RpcConnection) {
		rpcConn.OnRequest(onReqeustHandle)
		text, err := rpcConn.Request([]byte("Welcome connected rpc Server ;)"))
		if err != nil {
			log.Printf("err: %v", err)
		}
		log.Printf("call client response: %s", text)
	})

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
