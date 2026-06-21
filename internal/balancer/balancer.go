package balancer

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"go.uber.org/zap"
)

type Strategy int

const (
	StrategyRoundRobin Strategy = iota
	StrategyWeightedRoundRobin
	StrategyLeastConnections
	StrategyIPHash
)

type Balancer struct {
	manager    *channel.Manager
	strategy   Strategy
	mu         sync.RWMutex
	logger     *zap.Logger
}

func NewBalancer(manager *channel.Manager, strategy Strategy, logger *zap.Logger) *Balancer {
	return &Balancer{
		manager:  manager,
		strategy: strategy,
		logger:   logger,
	}
}

func (b *Balancer) Read(p []byte) (int, error) {
	ch := b.selectReadChannel()
	if ch == nil {
		return 0, io.ErrClosedPipe
	}

	n, err := ch.Read(p)
	if err != nil {
		b.logger.Warn("read channel error",
			zap.Uint16("id", ch.ID),
			zap.Error(err))
		b.manager.RemoveChannel(ch.ID)
		return b.Read(p)
	}

	return n, nil
}

func (b *Balancer) Write(p []byte) (int, error) {
	ch := b.selectWriteChannel()
	if ch == nil {
		return 0, io.ErrClosedPipe
	}

	n, err := ch.Write(p)
	if err != nil {
		b.logger.Warn("write channel error",
			zap.Uint16("id", ch.ID),
			zap.Error(err))
		b.manager.RemoveChannel(ch.ID)
		return b.Write(p)
	}

	return n, nil
}

func (b *Balancer) selectReadChannel() *channel.Channel {
	switch b.strategy {
	case StrategyRoundRobin:
		return b.manager.GetNextReadChannel()
	case StrategyWeightedRoundRobin:
		return b.weightedSelect(b.manager.GetNextReadChannel)
	case StrategyLeastConnections:
		return b.leastConnectionsSelect(b.manager.GetNextReadChannel)
	case StrategyIPHash:
		return b.manager.GetNextReadChannel()
	default:
		return b.manager.GetNextReadChannel()
	}
}

func (b *Balancer) selectWriteChannel() *channel.Channel {
	switch b.strategy {
	case StrategyRoundRobin:
		return b.manager.GetNextWriteChannel()
	case StrategyWeightedRoundRobin:
		return b.weightedSelect(b.manager.GetNextWriteChannel)
	case StrategyLeastConnections:
		return b.leastConnectionsSelect(b.manager.GetNextWriteChannel)
	case StrategyIPHash:
		return b.manager.GetNextWriteChannel()
	default:
		return b.manager.GetNextWriteChannel()
	}
}

func (b *Balancer) weightedSelect(getter func() *channel.Channel) *channel.Channel {
	ch := getter()
	if ch == nil {
		return nil
	}

	stats := ch.GetStats()
	if stats.BytesSent+stats.BytesRecv == 0 {
		return ch
	}

	// Simple weight-based selection
	// Higher weight = more likely to be selected
	if ch.Weight > 0.5 {
		return ch
	}

	// Try to find a better channel
	for i := 0; i < 3; i++ {
		if candidate := getter(); candidate != nil {
			if candidate.Weight > ch.Weight {
				return candidate
			}
		}
	}

	return ch
}

func (b *Balancer) leastConnectionsSelect(getter func() *channel.Channel) *channel.Channel {
	ch := getter()
	if ch == nil {
		return nil
	}

	stats := ch.GetStats()
	if stats.PacketsSent+stats.PacketsRecv < 100 {
		return ch
	}

	// Find channel with least activity
	var best *channel.Channel
	var bestScore uint64

	for i := 0; i < 5; i++ {
		if candidate := getter(); candidate != nil {
			candidateStats := candidate.GetStats()
			score := candidateStats.PacketsSent + candidateStats.PacketsRecv
			if best == nil || score < bestScore {
				best = candidate
				bestScore = score
			}
		}
	}

	if best != nil && bestScore < stats.PacketsSent+stats.PacketsRecv {
		return best
	}

	return ch
}

func (b *Balancer) GetStats() map[uint16]channel.Stats {
	return b.manager.GetStats()
}

type HealthMonitor struct {
	balancer  *Balancer
	interval  time.Duration
	threshold float64
	logger    *zap.Logger
	stopCh    chan struct{}
}

func NewHealthMonitor(balancer *Balancer, interval time.Duration, threshold float64, logger *zap.Logger) *HealthMonitor {
	return &HealthMonitor{
		balancer:  balancer,
		interval:  interval,
		threshold: threshold,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

func (h *HealthMonitor) Start() {
	go h.monitor()
}

func (h *HealthMonitor) Stop() {
	close(h.stopCh)
}

func (h *HealthMonitor) monitor() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkHealth()
		}
	}
}

func (h *HealthMonitor) checkHealth() {
	stats := h.balancer.GetStats()

	for id, stat := range stats {
		if stat.PacketLoss > h.threshold {
			h.logger.Warn("high packet loss detected",
				zap.Uint16("channel", id),
				zap.Float64("loss", stat.PacketLoss))
		}

		if stat.Latency > 100*time.Millisecond {
			h.logger.Warn("high latency detected",
				zap.Uint16("channel", id),
				zap.Duration("latency", stat.Latency))
		}
	}
}

type ChannelPool struct {
	minChannels int
	maxChannels int
	current     int32
	manager     *channel.Manager
	mu          sync.RWMutex
	logger      *zap.Logger
}

func NewChannelPool(min, max int, manager *channel.Manager, logger *zap.Logger) *ChannelPool {
	return &ChannelPool{
		minChannels: min,
		maxChannels: max,
		manager:     manager,
		logger:      logger,
	}
}

func (p *ChannelPool) Acquire() bool {
	current := atomic.LoadInt32(&p.current)
	if current >= int32(p.maxChannels) {
		return false
	}

	atomic.AddInt32(&p.current, 1)
	return true
}

func (p *ChannelPool) Release() {
	atomic.AddInt32(&p.current, -1)
}

func (p *ChannelPool) Current() int {
	return int(atomic.LoadInt32(&p.current))
}

func (p *ChannelPool) ScaleUp() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	current := int(atomic.LoadInt32(&p.current))
	if current >= p.maxChannels {
		return false
	}

	atomic.AddInt32(&p.current, 1)
	p.logger.Info("scaled up channel pool",
		zap.Int("current", current+1),
		zap.Int("max", p.maxChannels))
	return true
}

func (p *ChannelPool) ScaleDown() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	current := int(atomic.LoadInt32(&p.current))
	if current <= p.minChannels {
		return false
	}

	atomic.AddInt32(&p.current, -1)
	p.logger.Info("scaled down channel pool",
		zap.Int("current", current-1),
		zap.Int("min", p.minChannels))
	return true
}
