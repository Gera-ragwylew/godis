package receiver

import "sync"

type RingBuffer[T any] struct {
	buffer []T
	head   int
	tail   int
	count  int
	mu     sync.RWMutex
}

func newRingBuffer[T any](size int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buffer: make([]T, size),
	}
}

func (rb *RingBuffer[T]) Write(data T) int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	written := 0
	rb.buffer[rb.tail] = data
	rb.tail = (rb.tail + 1) % len(rb.buffer)
	rb.count++
	written++

	return written
}

func (rb *RingBuffer[T]) Read(out []T) int {
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

func (rb *RingBuffer[T]) Available() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

func (rb *RingBuffer[T]) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.tail = 0
	rb.count = 0
}
