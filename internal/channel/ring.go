package channel

import (
	"sync"
)

type RingBuffer struct {
	data     []byte
	head     int
	tail     int
	size     int
	count    int
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
}

func NewRingBuffer(size int) *RingBuffer {
	rb := &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
	rb.notEmpty = sync.NewCond(&rb.mu)
	rb.notFull = sync.NewCond(&rb.mu)
	return rb
}

func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := len(p)
	if n > rb.size-rb.count {
		n = rb.size - rb.count
	}

	for i := 0; i < n; i++ {
		rb.data[rb.head] = p[i]
		rb.head = (rb.head + 1) % rb.size
	}
	rb.count += n
	rb.notEmpty.Signal()
	return n, nil
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for rb.count == 0 {
		rb.notEmpty.Wait()
	}

	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	for i := 0; i < n; i++ {
		p[i] = rb.data[rb.tail]
		rb.tail = (rb.tail + 1) % rb.size
	}
	rb.count -= n
	rb.notFull.Signal()
	return n, nil
}

func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
