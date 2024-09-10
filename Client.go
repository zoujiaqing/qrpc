package qrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

type RequestMessageHandler func(data []byte) []byte
type ConnectedHandler func(*Client)

type Client struct {
	addr              string
	port              uint
	RetryDelay        time.Duration
	conn              *RpcConnection
	connected         bool
	connectMu         sync.Mutex
	onMessageHanle    RequestMessageHandler
	onConnectedHandle ConnectedHandler
	reconnectCh       chan struct{}
	reconnectErr      chan error
	pingInterval      time.Duration
	pingTimer         *time.Timer
	pingValue         int
	timeout           uint   // 超时时间（秒）
	localIP           string // 本地网卡 IP
}

func NewClient(addr string, port uint) *Client {
	client := &Client{
		addr:         addr,
		port:         port,
		RetryDelay:   1, // 如果断联1秒后重试
		reconnectCh:  make(chan struct{}),
		reconnectErr: make(chan error),
		pingInterval: 1 * time.Second, // 默认每 1 秒发送一次 Ping 请求
		pingValue:    -1,              // 默认是 -1 不通状态
		timeout:      15,              // 默认超时时间 15 秒
		connected:    false,
	}

	return client
}

func (c *Client) stopPing() {
	log.Printf("Stop timer for ping.")
	c.pingValue = -1
	c.pingTimer.Stop()
}

func (c *Client) SetTimeout(timeout uint) {
	c.timeout = timeout
}

func (c *Client) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Client) OnConnect(handle ConnectedHandler) {
	c.onConnectedHandle = handle
}

func (c *Client) OnRequest(handle RequestMessageHandler) {
	c.onMessageHanle = handle
}

func createCustomPacketConn(localIP string) (net.PacketConn, error) {
	localAddr := &net.UDPAddr{
		IP: net.ParseIP(localIP),
	}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *Client) BindLocalIP(localIP string) {
	c.localIP = localIP
}

func (c *Client) Connect() error {
	c.connectMu.Lock()

	defer func() {
		c.connectMu.Unlock()
	}()

	if c.connected {
		log.Printf("can't allow repeat connect.")
		return errors.New("can't allow repeat connect.")
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"qrpc"},
	}

	quicConf := &quic.Config{
		MaxIncomingStreams:    1e10, // bidirectional streams
		MaxIncomingUniStreams: 1e10, // unidirectional streams
	}

	serverAddr := fmt.Sprintf("%s:%d", c.addr, c.port)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var conn quic.Connection
	var err error

	// Custom localIP
	if c.localIP != "" {
		log.Printf("Use bind localIP: %s", c.localIP)
		// Use custom dialer to specify local address
		packetConn, err := createCustomPacketConn(c.localIP)
		if err != nil {
			log.Printf("Failed to create packet connection: %v", err)
			return err
		}

		// Resolve server address
		serverUDPAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			log.Printf("Failed to resolve server address: %v", err)
			return err
		}

		// Dial QUIC session
		conn, err = quic.Dial(ctx, packetConn, serverUDPAddr, tlsConf, quicConf)
		if err != nil {
			log.Printf("Failed to connect server address: %s, localIP: %s, error: %v", serverAddr, c.localIP, err)
			return err
		}

		log.Printf("Server connected! address: %s, localIP: %s", serverAddr, c.localIP)
	} else {
		conn, err = quic.DialAddr(
			ctx,
			serverAddr,
			tlsConf,
			quicConf)

		if err != nil {
			log.Printf("Connected server address: %s, error: %v", serverAddr, err)
			return err
		}

		log.Printf("Server connected! address: %s", serverAddr)
	}

	c.conn = NewRpcConnection(1, conn.Context(), conn, c.timeout)
	c.conn.OnClose(c.handleConnectionClosed)
	c.conn.OnRequest(c.handleMessage)

	c.connected = true
	c.ping()
	// 启动定时器，定期发送 Ping 请求
	c.startPingTimer()

	if c.onConnectedHandle != nil {
		c.onConnectedHandle(c)
	}

	return nil
}

func (c *Client) Request(data []byte) ([]byte, error) {
	return c.conn.Request(data)
}

func (c *Client) GetPingValue() int {
	return c.pingValue
}

func (c *Client) startPingTimer() {
	log.Printf("Start timer for ping.")
	c.pingTimer = time.NewTimer(c.pingInterval)
	go func() {
		for {
			select {
			case <-c.pingTimer.C:
				c.ping()
				c.pingTimer.Reset(c.pingInterval) // 重新设置定时器
			case <-c.reconnectCh:
				// 停止定时器
				c.stopPing()
				return
			}
		}
	}()
}

func (c *Client) ping() {
	pingValue, err := c.conn.Ping()
	if err != nil {
		log.Printf("Ping error: %v", err)
		c.pingValue = -1
		// 处理 Ping 错误，可以重连或者其他处理
	} else {
		c.pingValue = pingValue
	}
}

func (c *Client) handleMessage(conn *RpcConnection, data []byte) []byte {
	if c.onMessageHanle != nil {
		return c.onMessageHanle(data)
	}
	return nil
}

func (c *Client) handleConnectionClosed(conn *RpcConnection) {
	select {
	case <-c.reconnectCh:
		c.connectMu.Lock()
		c.connected = false
		c.connectMu.Unlock()
		return // 收到断开连接信号，停止重连
	default:
		c.connectMu.Lock()
		c.connected = false
		c.connectMu.Unlock()

		// 停止定时器
		c.stopPing()
	}
}

func (c *Client) Disconnect() {
	// 关闭连接，并停止重连
	c.conn.Close()
	close(c.reconnectCh)
}
