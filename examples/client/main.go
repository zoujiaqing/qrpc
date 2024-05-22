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

	// bind local IP
	client.BindLocalIP("10.10.101.1")

	if err := client.Connect(); err != nil {
		return err
	}

	client.OnRequest(func(data []byte) []byte {
		return append([]byte("From Client Respond "), data...)
	})

	// go func() {
	// 	var i uint64 = 1
	// 	for {
	// 		data, err := client.Request([]byte("Hello1"))
	// 		if err != nil {
	// 			log.Printf("Request error: %v", err)
	// 		}
	// 		log.Printf("Respond1(%d): %s", i, string(data))
	// 		time.Sleep(10 * time.Millisecond)
	// 		i++
	// 	}
	// }()

	// go func() {
	// 	var i uint64 = 1
	// 	for {
	// 		data, err := client.Request([]byte("Hello2"))
	// 		if err != nil {
	// 			log.Printf("Request error: %v", err)
	// 		}
	// 		log.Printf("Respond2(%d): %s", i, string(data))
	// 		time.Sleep(10 * time.Millisecond)
	// 		i++
	// 	}
	// }()

	data, err := client.Request([]byte("Hello"))
	if err != nil {
		log.Printf("Request error: %v", err)
	}
	log.Printf("Respond: %s", string(data))

	for {
		// 输出 ping 值
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
