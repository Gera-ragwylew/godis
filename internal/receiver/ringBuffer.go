package receiver

import "sync"

type ringBuffer struct {
	buffer []int16
	head   int
	tail   int
	count  int
	mu     sync.RWMutex
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		buffer: make([]int16, size),
	}
}

func (rb *ringBuffer) Write(data []int16) int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	written := 0
	for i := 0; i < len(data) && rb.count < len(rb.buffer); i++ {
		rb.buffer[rb.tail] = data[i]
		rb.tail = (rb.tail + 1) % len(rb.buffer)
		rb.count++
		written++
	}
	return written
}

func (rb *ringBuffer) Read(out []int16) int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := 0
	for i := 0; i < len(out) && rb.count > 0; i++ {
		out[i] = rb.buffer[rb.head]
		rb.head = (rb.head + 1) % len(rb.buffer)
		rb.count--
		n++
	}
	return n
}

func (rb *ringBuffer) Available() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

func (rb *ringBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.tail = 0
	rb.count = 0
}
