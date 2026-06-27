package tun

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/compress"
	"go.uber.org/zap"
)

type Tunnel struct {
	iface      *Interface
	manager    *channel.Manager
	compressor compress.Compressor
	mtu        int
	logger     *zap.Logger
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}

	toChannels *channel.RingBuffer
	fromChannels *channel.RingBuffer
}

func NewTunnel(iface *Interface, manager *channel.Manager, compressor compress.Compressor, mtu int, logger *zap.Logger) *Tunnel {
	return &Tunnel{
		iface:      iface,
		manager:    manager,
		compressor: compressor,
		mtu:        mtu,
		logger:     logger,
		stopCh:     make(chan struct{}),
		toChannels: channel.NewRingBuffer(mtu * 100),
		fromChannels: channel.NewRingBuffer(mtu * 100),
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
	buf := make([]byte, t.mtu+100)
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		n, err := t.iface.Read(buf)
		if err != nil {
			if err != io.EOF {
				t.logger.Error("failed to read from TUN", zap.Error(err))
			}
			continue
		}

		if n == 0 {
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		t.toChannels.Write(data)
	}
}

func (t *Tunnel) writeToTUN() {
	buf := make([]byte, t.mtu+100)
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		n, err := t.fromChannels.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		if _, err := t.iface.Write(buf[:n]); err != nil {
			t.logger.Error("failed to write to TUN", zap.Error(err))
		}
	}
}

func (t *Tunnel) writeToChannels() {
	buf := make([]byte, t.mtu+100)
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		n, err := t.toChannels.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		ch := t.manager.GetNextWriteChannel()
		if ch == nil {
			time.Sleep(time.Millisecond)
			continue
		}

		if _, werr := ch.Write(buf[:n]); werr != nil {
			t.logger.Error("failed to write to channel",
				zap.Uint16("channel_id", ch.ID),
				zap.Error(werr))
			t.manager.RemoveChannel(ch.ID)
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
		}
	}
}

func (t *Tunnel) readFromChannel(ch *channel.Channel) {
	buf := make([]byte, t.mtu+100)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			if err != io.EOF {
				t.logger.Error("failed to read from channel",
					zap.Uint16("channel_id", ch.ID),
					zap.Error(err))
			}
			return
		}

		if n == 0 {
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		t.fromChannels.Write(data)
	}
}

func (t *Tunnel) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
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
