package qrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type RequestMessageHandler func(data []byte) []byte

type Client struct {
	addr           string
	port           uint
	RetryDelay     time.Duration
	conn           *RpcConnection
	onMessageHanle RequestMessageHandler
	reconnectCh    chan struct{}
	reconnectErr   chan error
	pingInterval   time.Duration
	pingTimer      *time.Timer
	pingValue      int
	timeout        uint   // 超时时间（秒）
	localIP        string // 本地网卡 IP
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
	}

	return client
}

func (c *Client) SetTimeout(timeout uint) {
	c.timeout = timeout
}

func (c *Client) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
			return err
		}
	} else {
		conn, err = quic.DialAddr(
			ctx,
			serverAddr,
			tlsConf,
			quicConf)

		if err != nil {
			return err
		}
	}

	c.conn = NewRpcConnection(1, conn.Context(), conn, c.timeout)
	c.conn.OnClose(c.handleConnectionClosed)
	c.conn.OnRequest(c.handleMessage)

	// 启动定时器，定期发送 Ping 请求
	c.startPingTimer()

	return nil
}

func (c *Client) Request(data []byte) ([]byte, error) {
	return c.conn.Request(data)
}

func (c *Client) GetPingValue() int {
	return c.pingValue
}

func (c *Client) startPingTimer() {
	c.pingTimer = time.NewTimer(c.pingInterval)
	go func() {
		for {
			select {
			case <-c.pingTimer.C:
				pingValue, err := c.conn.Ping()
				if err != nil {
					log.Printf("Ping error: %v", err)
					c.pingValue = -1
					// 处理 Ping 错误，可以重连或者其他处理
				} else {
					c.pingValue = pingValue
				}
				c.pingTimer.Reset(c.pingInterval) // 重新设置定时器
			case <-c.reconnectCh:
				// 停止定时器
				c.pingTimer.Stop()
				return
			}
		}
	}()
}

func (c *Client) handleMessage(conn *RpcConnection, data []byte) []byte {
	if c.onMessageHanle != nil {
		return c.onMessageHanle(data)
	}
	return nil
}

func (c *Client) handleConnectionClosed(conn *RpcConnection) {
	// 连接关闭时触发重连
	for {
		select {
		case <-c.reconnectCh:
			return // 收到断开连接信号，停止重连
		default:
			time.Sleep(c.RetryDelay)

			err := c.Connect()
			if err == nil {
				// 重连成功
				return
			}

			// 重连失败，继续尝试
		}
	}
}

func (c *Client) Disconnect() {
	// 关闭连接，并停止重连
	c.conn.Close()
	close(c.reconnectCh)
}
