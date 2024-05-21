package qrpc

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type MessageHandler func(*RpcConnection, []byte) []byte
type ClosedHandler func(*RpcConnection)

type RpcConnection struct {
	id             uint64
	ctx            context.Context
	conn           quic.Connection
	broker         *MessageBroker
	onCloseHandle  ClosedHandler
	onMessageHanle MessageHandler
	metadata       map[string]string
	remoteAddr     net.Addr
}

func NewRpcConnection(id uint64, ctx context.Context, conn quic.Connection, timeout uint) *RpcConnection {
	broker := NewMessageBroker(time.Second * time.Duration(timeout))
	connection := &RpcConnection{
		id:         id,
		ctx:        ctx,
		conn:       conn,
		broker:     broker,
		metadata:   make(map[string]string),
		remoteAddr: conn.RemoteAddr(),
	}
	connection.startReceive()
	return connection
}

func (c *RpcConnection) ID() uint64 {
	return c.id
}

func (c *RpcConnection) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *RpcConnection) AddMetadata(key, value string) {
	c.metadata[key] = value
}

func (c *RpcConnection) RemoveMetadata(key string) {
	delete(c.metadata, key)
}

func (c *RpcConnection) GetMetadata(key string) (string, bool) {
	value, ok := c.metadata[key]
	return value, ok
}

func (c *RpcConnection) OnClose(handle ClosedHandler) {
	c.onCloseHandle = handle
}

// Close 关闭连接
func (c *RpcConnection) Close() {
	if c.onCloseHandle != nil {
		c.onCloseHandle(c) // 调用 onClose 回调
	}
	if err := c.conn.CloseWithError(0, "connection closed"); err != nil {
		log.Printf("Failed to close connection: %v", err)
	}
}

func (c *RpcConnection) Ping() (int, error) {

	message := Message{
		ID:        1, // 消息ID为 1 是 Ping 方法
		IsAck:     false,
		Timestamp: time.Now().Unix(),
	}

	// 发送 ping 请求并记录返回时间
	start := time.Now()
	// TODO: ping 请求添加一个合适的超时时间
	_, err := c.sendRequest(message)
	if err != nil {
		return 0, err
	}
	elapsed := time.Since(start)

	return int(elapsed.Microseconds()), nil
}

func (c *RpcConnection) Request(data []byte) ([]byte, error) {

	// 创建一个消息对象
	message := Message{
		ID:        c.broker.idGenerator.Next(), // 使用 IDGenerator 生成唯一的消息ID
		IsAck:     false,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	return c.sendRequest(message)
}

func (c *RpcConnection) OnRequest(handle MessageHandler) {
	c.onMessageHanle = handle
}

func (c *RpcConnection) sendRequest(message Message) ([]byte, error) {
	// 创建一个用于接收 response 的 channel
	responseCh, errorCh := make(chan *Message, 1), make(chan error, 1)

	// 打开一个新的流
	stream, err := c.conn.OpenStream()
	if err != nil {
		return nil, err
	}

	// 异步发送消息并等待回复
	go func() {
		defer stream.Close()

		// 将消息写入到流中
		if err := message.Write(stream); err != nil {
			log.Printf("Write message error: %v", err)
			errorCh <- err
			return
		}

		// 等待回复
		response, err := c.broker.Send(&message)
		if err != nil {
			log.Printf("Error sending request: %v", err)
			errorCh <- err
			return
		}

		// 将 response 发送到 channel
		responseCh <- response
	}()

	// 等待 response 并处理
	select {
	case response := <-responseCh:
		return response.Data, nil
	case err := <-errorCh:
		return nil, err
	}
}

func (c *RpcConnection) respondRequest(message *Message) {
	if c.onMessageHanle == nil {
		log.Printf("Message handler is nil")
		return
	}

	// 打开一个新的流
	stream, err := c.conn.OpenStreamSync(c.ctx)
	if err != nil {
		log.Printf("Open stream error: %v", err)
		c.Close()
		return
	}

	var data []byte
	if message.ID < initialIDValue {
		// TODO: 处理 ping 之类的内部消息
		data = nil
	} else {
		data = c.onMessageHanle(c, message.Data)
	}

	responseMessage := &Message{ID: message.ID, IsAck: true, Data: data, Timestamp: time.Now().Unix()}

	// 将消息写入到流中
	if err := responseMessage.Write(stream); err != nil {
		log.Printf("Write message error: %v", err)
		_ = stream.Close()
		c.Close()
		return
	}

	_ = stream.Close()
}

func (c *RpcConnection) handleMessage(message *Message) {
	// 如果这个消息是个应答消息就调用 broker 去配对
	if message.IsAck {
		// 将接收到的消息传递给 MessageBroker 进行处理
		c.broker.ReceiveResponse(message)
	} else {
		c.respondRequest(message)
	}
}

// 系统接收
func (c *RpcConnection) startReceive() {
	go func() {
		for {
			// 接收到新的流
			stream, err := c.conn.AcceptStream(c.ctx)
			if err != nil {
				log.Printf("Accept stream error: %v", err)
				c.Close()
				return
			}

			// 异步处理每个流
			go func(stream quic.Stream) {
				defer stream.Close()
				// 读取消息
				var message Message
				if err := message.Read(stream); err != nil {
					log.Printf("Read message error: %v", err)
					c.Close()
					return
				}

				// 处理接收到的消息
				c.handleMessage(&message)
			}(stream)
		}
	}()
}
