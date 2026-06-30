package tun

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/balancer"
	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/compress"
	"go.uber.org/zap"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 1600)
		return &b
	},
}

func getBuf() *[]byte {
	return bufPool.Get().(*[]byte)
}

func putBuf(b *[]byte) {
	if cap(*b) <= 1600 {
		*b = (*b)[:0]
		bufPool.Put(b)
	}
}

type Tunnel struct {
	iface      *Interface
	manager    *channel.Manager
	balancer   *balancer.Balancer
	compressor compress.Compressor
	mtu        int
	logger     *zap.Logger
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}

	toChannels   chan []byte
	fromChannels chan []byte

	droppedIn  uint64
	droppedOut uint64
}

func NewTunnel(iface *Interface, manager *channel.Manager, compressor compress.Compressor, mtu int, logger *zap.Logger) *Tunnel {
	bal := balancer.NewBalancer(manager, balancer.StrategyWeightedRoundRobin, logger)
	return &Tunnel{
		iface:      iface,
		manager:    manager,
		balancer:   bal,
		compressor: compressor,
		mtu:        mtu,
		logger:     logger,
		stopCh:     make(chan struct{}),
		toChannels:   make(chan []byte, 8192),
		fromChannels: make(chan []byte, 8192),
	}
}

func (t *Tunnel) Start() error {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	t.logger.Info("starting tunnel",
		zap.String("tun", t.iface.Name()),
		zap.Int("mtu", t.mtu))

	go t.readFromTUN()
	go t.writeToTUN()
	go t.readFromChannels()
	go t.writeToChannels()

	return nil
}

func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	t.running = false
	close(t.stopCh)
	t.logger.Info("tunnel stopped")
}

func (t *Tunnel) readFromTUN() {
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		bufp := getBuf()
		buf := (*bufp)[:t.mtu+100]
		n, err := t.iface.Read(buf)
		if err != nil {
			putBuf(bufp)
			if err != io.EOF {
				t.logger.Error("failed to read from TUN", zap.Error(err))
			}
			continue
		}

		if n == 0 {
			putBuf(bufp)
			continue
		}

		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		putBuf(bufp)
		select {
		case t.toChannels <- pkt:
		default:
			atomic.AddUint64(&t.droppedIn, 1)
		}
	}
}

func (t *Tunnel) writeToTUN() {
	for {
		select {
		case <-t.stopCh:
			return
		case pkt := <-t.fromChannels:
			if _, err := t.iface.Write(pkt); err != nil {
				t.logger.Error("failed to write to TUN", zap.Error(err))
			}
		}
	}
}

func (t *Tunnel) writeToChannels() {
	for {
		select {
		case <-t.stopCh:
			return
		case pkt := <-t.toChannels:
			n, err := t.balancer.Write(pkt)
			if err != nil {
				atomic.AddUint64(&t.droppedOut, 1)
			}
			_ = n
		}
	}
}

func (t *Tunnel) readFromChannels() {
	channels := t.manager.GetReadChannels()
	for _, ch := range channels {
		go t.readFromChannel(ch)
	}

	for {
		select {
		case <-t.stopCh:
			return
		case <-time.After(500 * time.Millisecond):
			newChannels := t.manager.GetReadChannels()
			for _, ch := range newChannels {
				found := false
				for _, existing := range channels {
					if existing.ID == ch.ID {
						found = true
						break
					}
				}
				if !found {
					channels = append(channels, ch)
					go t.readFromChannel(ch)
				}
			}

			alive := channels[:0]
			for _, ch := range channels {
				if ch.IsHealthy(60 * time.Second) {
					alive = append(alive, ch)
				}
			}
			channels = alive
		}
	}
}

func (t *Tunnel) readFromChannel(ch *channel.Channel) {
	for {
		bufp := getBuf()
		buf := (*bufp)[:t.mtu+100]
		n, err := ch.Read(buf)
		if err != nil {
			putBuf(bufp)
			if err != io.EOF {
				t.logger.Error("failed to read from channel",
					zap.Uint16("channel_id", ch.ID),
					zap.Error(err))
			}
			return
		}

		if n == 0 {
			putBuf(bufp)
			continue
		}

		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		putBuf(bufp)
		select {
		case t.fromChannels <- pkt:
		default:
			atomic.AddUint64(&t.droppedOut, 1)
		}
	}
}

func (t *Tunnel) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

func (t *Tunnel) GetDropped() (in, out uint64) {
	return atomic.LoadUint64(&t.droppedIn), atomic.LoadUint64(&t.droppedOut)
}

type TunnelPool struct {
	tunnels []*Tunnel
	mu      sync.RWMutex
	logger  *zap.Logger
}

func NewTunnelPool(logger *zap.Logger) *TunnelPool {
	return &TunnelPool{
		tunnels: make([]*Tunnel, 0),
		logger:  logger,
	}
}

func (p *TunnelPool) Add(tunnel *Tunnel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunnels = append(p.tunnels, tunnel)
}

func (p *TunnelPool) Remove(tunnel *Tunnel) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, t := range p.tunnels {
		if t == tunnel {
			t.Stop()
			p.tunnels = append(p.tunnels[:i], p.tunnels[i+1:]...)
			return
		}
	}
}

func (p *TunnelPool) StartAll() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, tunnel := range p.tunnels {
		if err := tunnel.Start(); err != nil {
			p.logger.Error("failed to start tunnel", zap.Error(err))
		}
	}
}

func (p *TunnelPool) StopAll() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, tunnel := range p.tunnels {
		tunnel.Stop()
	}
}

type RouteEntry struct {
	Dest    *net.IPNet
	Gateway net.IP
	Metric  int
}

type RouteTable struct {
	routes []RouteEntry
	mu     sync.RWMutex
}

func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes: make([]RouteEntry, 0),
	}
}

func (rt *RouteTable) AddRoute(dest *net.IPNet, gateway net.IP, metric int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.routes = append(rt.routes, RouteEntry{
		Dest:    dest,
		Gateway: gateway,
		Metric:  metric,
	})
}

func (rt *RouteTable) RemoveRoute(dest *net.IPNet) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	for i, route := range rt.routes {
		if route.Dest.String() == dest.String() {
			rt.routes = append(rt.routes[:i], rt.routes[i+1:]...)
			return
		}
	}
}

func (rt *RouteTable) MatchRoute(dest net.IP) *RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var best *RouteEntry
	for i := range rt.routes {
		if rt.routes[i].Dest.Contains(dest) {
			if best == nil || rt.routes[i].Metric < best.Metric {
				best = &rt.routes[i]
			}
		}
	}
	return best
}

func (rt *RouteTable) GetRoutes() []RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	routes := make([]RouteEntry, len(rt.routes))
	copy(routes, rt.routes)
	return routes
}
