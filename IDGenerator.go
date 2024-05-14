package qrpc

import (
	"sync"
)

// 最小初始值
const initialIDValue = uint64(1000)

// IDGenerator 用于生成唯一的消息ID
type IDGenerator struct {
	mu  sync.Mutex
	seq uint64 // 1000 开始，1000 以内为预留 ID
}

// Next 生成下一个唯一的消息ID
func (m *IDGenerator) Next() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seq < initialIDValue || m.seq == ^uint64(0) { // 如果初始值小于预留范围或当前值即将达到最大值
		m.seq = initialIDValue // 重置为初始值
	} else {
		m.seq++ // 递增
	}

	return m.seq
}
