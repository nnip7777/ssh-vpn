package channel

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type ChannelType uint8

const (
	ChannelRead    ChannelType = 0x01
	ChannelWrite   ChannelType = 0x02
	ChannelControl ChannelType = 0x03
)

type State int32

const (
	StateIdle     State = 0
	StateActive   State = 1
	StateDegraded State = 2
	StateFailed   State = 3
)

type Stats struct {
	BytesSent    uint64
	BytesRecv    uint64
	PacketsSent  uint64
	PacketsRecv  uint64
	Latency      time.Duration
	PacketLoss   float64
	LastActivity time.Time
}

type Channel struct {
	ID     uint16
	Type   ChannelType
	Reader io.ReadCloser
	Writer io.WriteCloser
	State  State
	Weight float64
	Stats  Stats

	mu     sync.RWMutex
	closed bool
	logger *zap.Logger
}

func NewChannel(id uint16, channelType ChannelType, reader io.ReadCloser, writer io.WriteCloser, logger *zap.Logger) *Channel {
	return &Channel{
		ID:       id,
		Type:     channelType,
		Reader:   reader,
		Writer:   writer,
		State:    StateIdle,
		Weight:   1.0,
		logger:   logger,
		Stats: Stats{
			LastActivity: time.Now(),
		},
	}
}

func (c *Channel) Read(p []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	n, err := c.Reader.Read(p)
	if n > 0 {
		atomic.AddUint64(&c.Stats.BytesRecv, uint64(n))
		atomic.AddUint64(&c.Stats.PacketsRecv, 1)
		c.Stats.LastActivity = time.Now()
	}
	return n, err
}

func (c *Channel) Write(p []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	n, err := c.Writer.Write(p)
	if n > 0 {
		atomic.AddUint64(&c.Stats.BytesSent, uint64(n))
		atomic.AddUint64(&c.Stats.PacketsSent, 1)
		c.Stats.LastActivity = time.Now()
	}
	return n, err
}

func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.State = StateFailed

	var errs []error
	if err := c.Reader.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := c.Writer.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (c *Channel) IsHealthy(timeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.State == StateFailed {
		return false
	}
	if c.Stats.PacketsSent == 0 && c.Stats.PacketsRecv == 0 {
		return true
	}
	if time.Since(c.Stats.LastActivity) > timeout {
		return false
	}
	return true
}

func (c *Channel) GetStats() Stats {
	return Stats{
		BytesSent:    atomic.LoadUint64(&c.Stats.BytesSent),
		BytesRecv:    atomic.LoadUint64(&c.Stats.BytesRecv),
		PacketsSent:  atomic.LoadUint64(&c.Stats.PacketsSent),
		PacketsRecv:  atomic.LoadUint64(&c.Stats.PacketsRecv),
		Latency:      c.Stats.Latency,
		PacketLoss:   c.Stats.PacketLoss,
		LastActivity: c.Stats.LastActivity,
	}
}

type Manager struct {
	ReadChannels    []*Channel
	WriteChannels   []*Channel
	ControlChannel  *Channel

	readMu    sync.RWMutex
	writeMu   sync.RWMutex
	controlMu sync.RWMutex

	readIndex  uint64
	writeIndex uint64

	MinRead    int
	MaxRead    int
	MinWrite   int
	MaxWrite   int
	ReadRatio  float64
	WriteRatio float64

	HealthCheck time.Duration
	Timeout     time.Duration
	logger      *zap.Logger

	TotalCreated uint64
	TotalClosed  uint64
	CreatedAt    time.Time
}

func NewManager(minRead, maxRead, minWrite, maxWrite int, readRatio, writeRatio float64, healthCheck, timeout time.Duration, logger *zap.Logger) *Manager {
	return &Manager{
		ReadChannels:  make([]*Channel, 0, maxRead),
		WriteChannels: make([]*Channel, 0, maxWrite),
		MinRead:       minRead,
		MaxRead:       maxRead,
		MinWrite:      minWrite,
		MaxWrite:      maxWrite,
		ReadRatio:     readRatio,
		WriteRatio:    writeRatio,
		HealthCheck:   healthCheck,
		Timeout:       timeout,
		logger:        logger,
		CreatedAt:     time.Now(),
	}
}

func (m *Manager) AddChannel(ch *Channel) {
	atomic.AddUint64(&m.TotalCreated, 1)
	switch ch.Type {
	case ChannelRead:
		m.readMu.Lock()
		m.ReadChannels = append(m.ReadChannels, ch)
		m.readMu.Unlock()
		m.logger.Info("added read channel",
			zap.Uint16("id", ch.ID),
			zap.Int("total", len(m.ReadChannels)))
	case ChannelWrite:
		m.writeMu.Lock()
		m.WriteChannels = append(m.WriteChannels, ch)
		m.writeMu.Unlock()
		m.logger.Info("added write channel",
			zap.Uint16("id", ch.ID),
			zap.Int("total", len(m.WriteChannels)))
	case ChannelControl:
		m.controlMu.Lock()
		m.ControlChannel = ch
		m.controlMu.Unlock()
		m.logger.Info("set control channel", zap.Uint16("id", ch.ID))
	}
}

func (m *Manager) RemoveChannel(id uint16) {
	atomic.AddUint64(&m.TotalClosed, 1)
	m.readMu.Lock()
	for i, ch := range m.ReadChannels {
		if ch.ID == id {
			m.ReadChannels = append(m.ReadChannels[:i], m.ReadChannels[i+1:]...)
			ch.Close()
			break
		}
	}
	m.readMu.Unlock()

	m.writeMu.Lock()
	for i, ch := range m.WriteChannels {
		if ch.ID == id {
			m.WriteChannels = append(m.WriteChannels[:i], m.WriteChannels[i+1:]...)
			ch.Close()
			break
		}
	}
	m.writeMu.Unlock()
}

func (m *Manager) GetNextReadChannel() *Channel {
	m.readMu.RLock()
	defer m.readMu.RUnlock()

	if len(m.ReadChannels) == 0 {
		return nil
	}

	for i := 0; i < len(m.ReadChannels); i++ {
		idx := atomic.AddUint64(&m.readIndex, 1) % uint64(len(m.ReadChannels))
		ch := m.ReadChannels[idx]
		if ch.IsHealthy(m.Timeout) {
			return ch
		}
	}

	return nil
}

func (m *Manager) GetReadChannels() []*Channel {
	m.readMu.RLock()
	defer m.readMu.RUnlock()

	result := make([]*Channel, len(m.ReadChannels))
	copy(result, m.ReadChannels)
	return result
}

func (m *Manager) GetNextWriteChannel() *Channel {
	m.writeMu.RLock()
	defer m.writeMu.RUnlock()

	if len(m.WriteChannels) == 0 {
		return nil
	}

	for i := 0; i < len(m.WriteChannels); i++ {
		idx := atomic.AddUint64(&m.writeIndex, 1) % uint64(len(m.WriteChannels))
		ch := m.WriteChannels[idx]
		if ch.IsHealthy(m.Timeout) {
			return ch
		}
	}

	return nil
}

func (m *Manager) GetControlChannel() *Channel {
	m.controlMu.RLock()
	defer m.controlMu.RUnlock()
	return m.ControlChannel
}

func (m *Manager) HealthCheckLoop() {
	ticker := time.NewTicker(m.HealthCheck)
	defer ticker.Stop()

	for range ticker.C {
		m.checkChannelHealth()
	}
}

func (m *Manager) checkChannelHealth() {
	m.readMu.Lock()
	healthy := make([]*Channel, 0, len(m.ReadChannels))
	for _, ch := range m.ReadChannels {
		if ch.IsHealthy(m.Timeout) {
			healthy = append(healthy, ch)
		} else {
			m.logger.Warn("removing unhealthy read channel",
				zap.Uint16("id", ch.ID))
			ch.Close()
		}
	}
	m.ReadChannels = healthy
	m.readMu.Unlock()

	m.writeMu.Lock()
	healthy = make([]*Channel, 0, len(m.WriteChannels))
	for _, ch := range m.WriteChannels {
		if ch.IsHealthy(m.Timeout) {
			healthy = append(healthy, ch)
		} else {
			m.logger.Warn("removing unhealthy write channel",
				zap.Uint16("id", ch.ID))
			ch.Close()
		}
	}
	m.WriteChannels = healthy
	m.writeMu.Unlock()
}

func (m *Manager) GetStats() map[uint16]Stats {
	stats := make(map[uint16]Stats)

	m.readMu.RLock()
	for _, ch := range m.ReadChannels {
		stats[ch.ID] = ch.GetStats()
	}
	m.readMu.RUnlock()

	m.writeMu.RLock()
	for _, ch := range m.WriteChannels {
		stats[ch.ID] = ch.GetStats()
	}
	m.writeMu.RUnlock()

	return stats
}

func (m *Manager) GetManagerStats() map[string]interface{} {
	m.readMu.RLock()
	activeRead := len(m.ReadChannels)
	m.readMu.RUnlock()

	m.writeMu.RLock()
	activeWrite := len(m.WriteChannels)
	m.writeMu.RUnlock()

	return map[string]interface{}{
		"active_read":   activeRead,
		"active_write":  activeWrite,
		"total_created": atomic.LoadUint64(&m.TotalCreated),
		"total_closed":  atomic.LoadUint64(&m.TotalClosed),
		"created_at":    m.CreatedAt,
		"min_read":      m.MinRead,
		"min_write":     m.MinWrite,
	}
}

func (m *Manager) ChannelCount() (read int, write int) {
	m.readMu.RLock()
	read = len(m.ReadChannels)
	m.readMu.RUnlock()

	m.writeMu.RLock()
	write = len(m.WriteChannels)
	m.writeMu.RUnlock()

	return
}

func (m *Manager) CloseAll() {
	m.readMu.Lock()
	for _, ch := range m.ReadChannels {
		ch.Close()
	}
	m.ReadChannels = m.ReadChannels[:0]
	m.readMu.Unlock()

	m.writeMu.Lock()
	for _, ch := range m.WriteChannels {
		ch.Close()
	}
	m.WriteChannels = m.WriteChannels[:0]
	m.writeMu.Unlock()

	m.controlMu.Lock()
	if m.ControlChannel != nil {
		m.ControlChannel.Close()
		m.ControlChannel = nil
	}
	m.controlMu.Unlock()
}
