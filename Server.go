package qrpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

type ConnectionHandler func(*RpcConnection)

type Server struct {
	listener           *quic.Listener
	connections        map[uint64]*RpcConnection
	mutex              sync.Mutex
	idGenerator        IDGenerator
	onConnectionHandle ConnectionHandler
	onClosedHandle     ClosedHandler
	timeout            uint
}

func NewServer(port uint) (*Server, error) {
	tlsConf := generateTLSConfig()

	listener, err := quic.ListenAddr(fmt.Sprintf(":%d", port), tlsConf, &quic.Config{
		KeepAlivePeriod:    10 * time.Second,
		MaxIncomingStreams: 1e10,
	})
	if err != nil {
		return nil, err
	}

	return &Server{
		listener:    listener,
		connections: map[uint64]*RpcConnection{},
		idGenerator: IDGenerator{seq: initialIDValue},
		timeout:     15,
	}, nil
}

func (s *Server) SetTimeout(timeout uint) {
	s.timeout = timeout
}

func (s *Server) OnConnection(handle ConnectionHandler) {
	s.onConnectionHandle = handle
}

func (s *Server) OnClose(handle ClosedHandler) {
	s.onClosedHandle = handle
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) Accept(ctx context.Context) {
	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			log.Printf("[ERROR] failed to accept new connection: %v\n", err)
			return
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn quic.Connection) {
	connID := s.idGenerator.Next()
	rpcConn := NewRpcConnection(connID, ctx, conn, s.timeout)
	rpcConn.OnClose(func(conn *RpcConnection) {
		s.removeConnection(connID)
		if s.onClosedHandle != nil {
			s.onClosedHandle(conn)
		}
	})

	s.mutex.Lock()
	s.connections[connID] = rpcConn
	s.mutex.Unlock()

	if s.onConnectionHandle != nil {
		s.onConnectionHandle(rpcConn)
	}
}

func (s *Server) removeConnection(connID uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.connections, connID)
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qrpc"},
	}
}
