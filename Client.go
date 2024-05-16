package qrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/quic-go/quic-go"
)

type RequestMessageHandler func(data []byte) []byte

type Client struct {
	Addr           string
	Port           uint
	RetryDelay     time.Duration
	conn           *RpcConnection
	onMessageHanle RequestMessageHandler
	reconnectCh    chan struct{}
	reconnectErr   chan error
	pingInterval   time.Duration
	pingTimer      *time.Timer
	pingValue      int
}

func NewClient(addr string, port uint) *Client {
	client := &Client{
		Addr:         addr,
		Port:         port,
		RetryDelay:   3, // 如果断联3秒后重试
		reconnectCh:  make(chan struct{}),
		reconnectErr: make(chan error),
		pingInterval: 3 * time.Second, // 默认每 1 秒发送一次 Ping 请求
		pingValue:    -1,              // 默认是 -1 不通状态
	}

	return client
}

func (c *Client) OnRequest(handle RequestMessageHandler) {
	c.onMessageHanle = handle
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

	conn, err := quic.DialAddr(
		context.Background(),
		fmt.Sprintf("%s:%d", c.Addr, c.Port),
		tlsConf,
		quicConf)

	if err != nil {
		return err
	}

	c.conn = NewRpcConnection(1, conn.Context(), conn, 3)
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

func (c *Client) handleConnectionClosed() {
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
