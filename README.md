# QRPC

A simple rpc framework that works over [QUIC](https://en.wikipedia.org/wiki/QUIC) written in Golang.

## Client sample code

```go
package main

import (
	"log"
	"time"

	"github.com/zoujiaqing/qrpc"
)

func main() {
	client := qrpc.NewClient("localhost", 4444)

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
			break
		}
		log.Printf("Respond(%d): %s", i, string(data))
		time.Sleep(1 * time.Second)
		i++
	}
}

```

## Server sample code

```go
package main

import (
	"context"

	"github.com/zoujiaqing/qrpc"
)

func main() {
	server, err := qrpc.NewServer(4444)
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server.Accept(ctx)
}

```

## Run examples
examples include grpc client and server sample code

### Load dependency

```bash
go mod dity
```

### Run server for example
```bash
go run ./examples/server
```

### Run client for example
```bash
go run ./examples/client
```
