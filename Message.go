package qrpc

// Message 表示一个消息
type Message struct {
	ID        uint64
	IsAck     bool
	Data      []byte
	Timestamp int64
}
