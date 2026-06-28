package channel

import (
	"io"
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
	closed   bool
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

	if rb.closed {
		return 0, io.ErrClosedPipe
	}

	n := len(p)
	if n > rb.size-rb.count {
		n = rb.size - rb.count
	}

	for n == 0 && !rb.closed {
		rb.notFull.Wait()
		n = len(p)
		if n > rb.size-rb.count {
			n = rb.size - rb.count
		}
	}

	if n > 0 {
		end := rb.head + n
		if end <= rb.size {
			copy(rb.data[rb.head:end], p[:n])
		} else {
			first := rb.size - rb.head
			copy(rb.data[rb.head:rb.size], p[:first])
			copy(rb.data[0:n-first], p[first:n])
		}
		rb.head = end % rb.size
		rb.count += n
	}
	rb.notEmpty.Signal()
	return n, nil
}

func (rb *RingBuffer) WriteNoBlock(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return 0, io.ErrClosedPipe
	}

	n := len(p)
	if n > rb.size-rb.count {
		n = rb.size - rb.count
	}

	if n > 0 {
		end := rb.head + n
		if end <= rb.size {
			copy(rb.data[rb.head:end], p[:n])
		} else {
			first := rb.size - rb.head
			copy(rb.data[rb.head:rb.size], p[:first])
			copy(rb.data[0:n-first], p[first:n])
		}
		rb.head = end % rb.size
		rb.count += n
	}
	rb.notEmpty.Signal()
	return n, nil
}

func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for rb.count == 0 {
		if rb.closed {
			return 0, io.ErrClosedPipe
		}
		rb.notEmpty.Wait()
	}

	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	if n > 0 {
		end := rb.tail + n
		if end <= rb.size {
			copy(p[:n], rb.data[rb.tail:end])
		} else {
			first := rb.size - rb.tail
			copy(p[:first], rb.data[rb.tail:rb.size])
			copy(p[first:n], rb.data[0:n-first])
		}
		rb.tail = end % rb.size
		rb.count -= n
	}
	rb.notFull.Signal()
	return n, nil
}

func (rb *RingBuffer) ReadTimeout(p []byte, maxWait int) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		if rb.closed {
			return 0, io.ErrClosedPipe
		}
		rb.notEmpty.Wait()
	}

	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	if n > 0 {
		end := rb.tail + n
		if end <= rb.size {
			copy(p[:n], rb.data[rb.tail:end])
		} else {
			first := rb.size - rb.tail
			copy(p[:first], rb.data[rb.tail:rb.size])
			copy(p[first:n], rb.data[0:n-first])
		}
		rb.tail = end % rb.size
		rb.count -= n
	}
	rb.notFull.Signal()
	return n, nil
}

func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *RingBuffer) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.notEmpty.Broadcast()
	rb.notFull.Broadcast()
}

func (rb *RingBuffer) FreeSpace() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size - rb.count
}
