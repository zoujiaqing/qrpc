package qrpc

import (
	"time"

	"encoding/gob"
	"errors"
	"io"
	"sync"
)

// Read 从 io.Reader 中读取消息数据
func (m *Message) Read(r io.Reader) error {
	return gob.NewDecoder(r).Decode(m)
}

// Write 将消息数据写入到 io.Writer 中
func (m *Message) Write(w io.Writer) error {
	return gob.NewEncoder(w).Encode(m)
}

// MessagePair 表示一个消息和其回复消息的配对
type MessagePair struct {
	Request  *Message
	Response *Message
}

// MessageBroker 负责管理消息配对和等待回复
type MessageBroker struct {
	mu          sync.Mutex
	messageMap  map[uint64]chan *Message
	timeout     time.Duration
	idGenerator IDGenerator
}

// NewMessageBroker 创建一个新的消息 Broker
func NewMessageBroker(timeout time.Duration) *MessageBroker {
	return &MessageBroker{
		messageMap:  make(map[uint64]chan *Message),
		timeout:     timeout,
		idGenerator: IDGenerator{seq: initialIDValue},
	}
}

// Send 发送消息并等待回复，如果超时返回错误
func (b *MessageBroker) Send(request *Message) (*Message, error) {
	// 创建一个用于接收回复的通道
	responseCh := make(chan *Message, 1)

	// 将通道与请求消息的ID关联起来
	b.mu.Lock()
	b.messageMap[request.ID] = responseCh
	b.mu.Unlock()

	// 发送消息
	// 这里假设你有一个发送消息的方法，例如 SendMessage(request *Message)

	// 等待回复或超时
	select {
	case response := <-responseCh:
		// 成功接收到回复，返回回复消息
		return response, nil
	case <-time.After(b.timeout):
		// 超时，删除关联的通道
		b.mu.Lock()
		delete(b.messageMap, request.ID)
		b.mu.Unlock()
		return nil, errors.New("timeout waiting for response")
	}
}

// ReceiveResponse 接收回复消息，并通知等待的请求方
func (b *MessageBroker) ReceiveResponse(response *Message) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 查找与回复消息关联的请求方通道
	if ch, ok := b.messageMap[response.ID]; ok {
		// 将回复消息发送到对应的通道
		ch <- response

		// 删除关联的通道
		delete(b.messageMap, response.ID)
		return true
	}

	return false
}
