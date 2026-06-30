package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/balancer"
	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/compress"
	"github.com/nnip7777/ssh-vpn/internal/config"
	sshtransport "github.com/nnip7777/ssh-vpn/internal/ssh"
	"github.com/nnip7777/ssh-vpn/internal/tun"
	"go.uber.org/zap"
)

type extraConn struct {
	transport   *sshtransport.Transport
	negotiator  *sshtransport.ChannelNegotiator
	channelMgr  *channel.Manager
	refreshStop chan struct{}
}

type Client struct {
	config       *config.Config
	sshConfig    *sshtransport.ClientConfig
	transport    *sshtransport.Transport
	handshake    *sshtransport.Handshake
	negotiator   *sshtransport.ChannelNegotiator
	channelMgr   *channel.Manager
	balancer     *balancer.Balancer
	tunIface     *tun.Interface
	tunnel       *tun.Tunnel
	compressor   compress.Compressor
	routeManager *tun.RouteManager
	logger       *zap.Logger
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	refreshStop  chan struct{}

	extraConns []*extraConn
}

func New(cfg *config.Config, logger *zap.Logger) (*Client, error) {
	sshCfg := &sshtransport.ClientConfig{
		ServerAddr:     fmt.Sprintf("%s:%d", cfg.Client.ServerAddr, cfg.Client.ServerPort),
		Username:       cfg.Client.Username,
		Password:       cfg.Client.Password,
		PrivateKeyPath: cfg.Client.PrivateKeyPath,
	}

	var comp compress.Compressor
	var err error
	switch cfg.Security.Compression {
	case "lz4":
		comp, err = compress.NewLZ4Compressor(logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}
	default:
		comp = compress.NewNoopCompressor()
	}

	channelMgr := channel.NewManager(
		cfg.Channels.MinRead,
		cfg.Channels.MaxRead,
		cfg.Channels.MinWrite,
		cfg.Channels.MaxWrite,
		cfg.Channels.ReadRatio,
		cfg.Channels.WriteRatio,
		cfg.Channels.HealthCheck,
		cfg.Channels.Timeout,
		logger,
	)

	tunIface, err := tun.New(tun.Config{
		Name:    cfg.Client.TUNName,
		Addr:    cfg.Client.TUNAddr,
		Peer:    computePeerAddr(cfg.Client.TUNAddr),
		Netmask: cfg.Client.TUNNetmask,
		MTU:     cfg.Client.MTU,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN interface: %w", err)
	}

	bal := balancer.NewBalancer(channelMgr, balancer.StrategyWeightedRoundRobin, logger)

	tunnel := tun.NewTunnel(tunIface, channelMgr, comp, cfg.Client.MTU, logger)

	routeSubnet := fmt.Sprintf("%s/24", cfg.Client.TUNAddr)
	routeManager := tun.NewRouteManager(
		tunIface.Name(),
		routeSubnet,
		cfg.Client.ServerAddr,
		logger,
	)

	return &Client{
		config:       cfg,
		sshConfig:    sshCfg,
		channelMgr:   channelMgr,
		balancer:     bal,
		tunIface:     tunIface,
		tunnel:       tunnel,
		compressor:   comp,
		routeManager: routeManager,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}, nil
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("connecting to server",
		zap.String("addr", fmt.Sprintf("%s:%d", c.config.Client.ServerAddr, c.config.Client.ServerPort)))

	sshCfg, err := sshtransport.NewSSHClientConfig(c.sshConfig, c.logger)
	if err != nil {
		return fmt.Errorf("failed to create SSH client config: %w", err)
	}

	transport := sshtransport.NewTransport(
		fmt.Sprintf("%s:%d", c.config.Client.ServerAddr, c.config.Client.ServerPort),
		sshCfg,
		c.logger,
	)

	if err := transport.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.transport = transport

	handshake := sshtransport.NewClientHandshake(transport.GetConnection(), c.logger)
	handshake.SetChannelRatios(c.config.Channels.ReadRatio, c.config.Channels.WriteRatio)
	handshake.SetChannelLimits(uint32(c.config.Channels.MinRead), uint32(c.config.Channels.MaxRead))

	if err := handshake.DoClientHandshake(); err != nil {
		transport.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	c.handshake = handshake

	negotiator := sshtransport.NewChannelNegotiator(handshake, c.channelMgr, c.config.Channels.MinWrite, c.logger)
	if err := negotiator.NegotiateChannels(); err != nil {
		transport.Close()
		return fmt.Errorf("channel negotiation failed: %w", err)
	}

	c.negotiator = negotiator

	if c.refreshStop != nil {
		close(c.refreshStop)
	}
	c.refreshStop = make(chan struct{})
	go negotiator.MonitorChannels(c.config.Channels.HealthCheck)
	go negotiator.ProactiveRefresh(c.refreshStop)

	transport.StartKeepalive(15 * time.Second)

	for _, extraPort := range c.config.Client.ExtraPorts {
		c.connectExtraPort(extraPort)
	}

	c.logger.Info("connected to server")
	return nil
}

func (c *Client) connectExtraPort(port int) {
	addr := fmt.Sprintf("%s:%d", c.config.Client.ServerAddr, port)
	c.logger.Info("connecting to extra port", zap.String("addr", addr))

	sshCfg, err := sshtransport.NewSSHClientConfig(c.sshConfig, c.logger)
	if err != nil {
		c.logger.Warn("failed to create SSH config for extra port", zap.Error(err))
		return
	}

	transport := sshtransport.NewTransport(addr, sshCfg, c.logger)
	if err := transport.Connect(); err != nil {
		c.logger.Warn("failed to connect to extra port", zap.Error(err))
		return
	}

	handshake := sshtransport.NewClientHandshake(transport.GetConnection(), c.logger)
	handshake.SetChannelRatios(c.config.Channels.ReadRatio, c.config.Channels.WriteRatio)
	handshake.SetChannelLimits(uint32(c.config.Channels.MinRead), uint32(c.config.Channels.MaxRead))

	if err := handshake.DoClientHandshake(); err != nil {
		transport.Close()
		c.logger.Warn("handshake failed on extra port", zap.Error(err))
		return
	}

	extraMgr := channel.NewManager(
		c.config.Channels.MinRead, c.config.Channels.MaxRead,
		c.config.Channels.MinWrite, c.config.Channels.MaxWrite,
		c.config.Channels.ReadRatio, c.config.Channels.WriteRatio,
		c.config.Channels.HealthCheck, c.config.Channels.Timeout,
		c.logger,
	)

	negotiator := sshtransport.NewChannelNegotiator(handshake, extraMgr, c.config.Channels.MinWrite, c.logger)
	if err := negotiator.NegotiateChannels(); err != nil {
		transport.Close()
		c.logger.Warn("channel negotiation failed on extra port", zap.Error(err))
		return
	}

	refreshStop := make(chan struct{})
	go negotiator.MonitorChannels(c.config.Channels.HealthCheck)
	go negotiator.ProactiveRefresh(refreshStop)

	transport.StartKeepalive(15 * time.Second)

	ec := &extraConn{
		transport:   transport,
		negotiator:  negotiator,
		channelMgr:  extraMgr,
		refreshStop: refreshStop,
	}
	c.extraConns = append(c.extraConns, ec)

	c.logger.Info("connected to extra port", zap.Int("port", port))
}

func (c *Client) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("client already running")
	}

	c.running = true

	if c.routeManager != nil {
		if err := c.routeManager.SaveAndSetup(); err != nil {
			c.logger.Warn("route setup failed", zap.Error(err))
		}
	}

	if err := c.tunnel.Start(); err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	go c.monitorConnection()

	c.logger.Info("client started")
	return nil
}

func (c *Client) monitorConnection() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.checkConnection()
		}
	}
}

func (c *Client) checkConnection() {
	c.mu.RLock()
	running := c.running
	transport := c.transport
	c.mu.RUnlock()

	if !running {
		return
	}

	if transport != nil && !transport.IsConnected() {
		c.logger.Warn("connection lost, attempting to reconnect")
		c.reconnect()
	}
}

func (c *Client) reconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Warn("connection lost, attempting to reconnect")

	if c.tunnel != nil {
		c.tunnel.Stop()
	}

	if c.transport != nil {
		c.transport.Close()
	}

	for _, ec := range c.extraConns {
		close(ec.refreshStop)
		ec.transport.Close()
	}
	c.extraConns = nil

	c.channelMgr.CloseAll()

	for i := 0; i < 5; i++ {
		c.logger.Info("attempting to reconnect",
			zap.Int("attempt", i+1))

		sshCfg, err := sshtransport.NewSSHClientConfig(c.sshConfig, c.logger)
		if err != nil {
			c.logger.Warn("failed to create SSH config for reconnect", zap.Error(err))
			if i >= 3 {
				time.Sleep(time.Duration(i-2) * time.Second)
			}
			continue
		}

		transport := sshtransport.NewTransport(
			fmt.Sprintf("%s:%d", c.config.Client.ServerAddr, c.config.Client.ServerPort),
			sshCfg,
			c.logger,
		)

		if err := transport.Connect(); err != nil {
			c.logger.Warn("reconnect failed", zap.Error(err))
			if i >= 3 {
				time.Sleep(time.Duration(i-2) * time.Second)
			}
			continue
		}

		handshake := sshtransport.NewClientHandshake(transport.GetConnection(), c.logger)
		handshake.SetChannelRatios(c.config.Channels.ReadRatio, c.config.Channels.WriteRatio)
		handshake.SetChannelLimits(uint32(c.config.Channels.MinRead), uint32(c.config.Channels.MaxRead))

		if err := handshake.DoClientHandshake(); err != nil {
			transport.Close()
			c.logger.Warn("handshake failed during reconnect", zap.Error(err))
			if i >= 3 {
				time.Sleep(time.Duration(i-2) * time.Second)
			}
			continue
		}

		negotiator := sshtransport.NewChannelNegotiator(handshake, c.channelMgr, c.config.Channels.MinWrite, c.logger)
		if err := negotiator.NegotiateChannels(); err != nil {
			transport.Close()
			c.logger.Warn("channel negotiation failed during reconnect", zap.Error(err))
			if i >= 3 {
				time.Sleep(time.Duration(i-2) * time.Second)
			}
			continue
		}

		c.negotiator = negotiator

		if c.refreshStop != nil {
			close(c.refreshStop)
		}
		c.refreshStop = make(chan struct{})
		go negotiator.MonitorChannels(c.config.Channels.HealthCheck)
		go negotiator.ProactiveRefresh(c.refreshStop)

		transport.StartKeepalive(15 * time.Second)

		c.transport = transport
		c.handshake = handshake

		for _, extraPort := range c.config.Client.ExtraPorts {
			c.connectExtraPort(extraPort)
		}

		c.tunnel = tun.NewTunnel(c.tunIface, c.channelMgr, c.compressor, c.config.Client.MTU, c.logger)
		if err := c.tunnel.Start(); err != nil {
			c.logger.Error("failed to restart tunnel after reconnect", zap.Error(err))
			transport.Close()
			if i >= 3 {
				time.Sleep(time.Duration(i-2) * time.Second)
			}
			continue
		}

		c.logger.Info("reconnected successfully")
		return
	}

	c.logger.Error("failed to reconnect after 5 attempts")
	c.Stop()
}

func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	close(c.stopCh)

	if c.refreshStop != nil {
		close(c.refreshStop)
		c.refreshStop = nil
	}

	for _, ec := range c.extraConns {
		close(ec.refreshStop)
		ec.transport.Close()
	}
	c.extraConns = nil

	if c.tunnel != nil {
		c.tunnel.Stop()
	}

	if c.transport != nil {
		c.transport.Close()
	}

	if c.routeManager != nil {
		c.routeManager.Restore()
	}

	if c.tunIface != nil {
		c.tunIface.Close()
	}

	c.logger.Info("client stopped")
}

func (c *Client) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make(map[string]interface{})

	if c.channelMgr != nil {
		channelStats := c.channelMgr.GetStats()
		readCount, writeCount := c.channelMgr.ChannelCount()
		stats["read_channels"] = readCount
		stats["write_channels"] = writeCount
		stats["channel_stats"] = channelStats
		stats["manager_stats"] = c.channelMgr.GetManagerStats()
	}

	if c.tunnel != nil {
		dropIn, dropOut := c.tunnel.GetDropped()
		stats["dropped_in"] = dropIn
		stats["dropped_out"] = dropOut
	}

	stats["running"] = c.running
	stats["connected"] = c.transport != nil && c.transport.IsConnected()

	return stats
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.transport != nil && c.transport.IsConnected()
}

func computePeerAddr(addr string) string {
	parts := net.ParseIP(addr).To4()
	if parts == nil {
		return addr
	}
	parts[3] = 1
	return parts.String()
}
