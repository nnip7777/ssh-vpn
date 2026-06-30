package lib

import (
	"fmt"
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

type VPNClient struct {
	config     *config.Config
	transport  *sshtransport.Transport
	handshake  *sshtransport.Handshake
	channelMgr *channel.Manager
	balancer   *balancer.Balancer
	tunIface   *tun.Interface
	tunnel     *tun.Tunnel
	compressor compress.Compressor
	logger     *zap.Logger
	mu         sync.RWMutex
	running    bool
	status     string
	onStatus   func(string)
	onError    func(string)
	onStats    func(map[string]interface{})
}

type VPNConfig struct {
	ServerAddr     string
	ServerPort     int
	Username       string
	Password       string
	PrivateKeyPath string
	TUNName        string
	TUNAddr        string
	TUNNetmask     string
	MTU            int
	MinRead        int
	MaxRead        int
	MinWrite       int
	MaxWrite       int
	ReadRatio      float64
	WriteRatio     float64
	Compression    string
}

func DefaultVPNConfig() *VPNConfig {
	return &VPNConfig{
		ServerAddr:  "localhost",
		ServerPort:  22,
		TUNName:     "tun0",
		TUNAddr:     "10.0.0.2",
		TUNNetmask:  "255.255.255.0",
		MTU:         1400,
		MinRead:     2,
		MaxRead:     8,
		MinWrite:    1,
		MaxWrite:    4,
		ReadRatio:   0.8,
		WriteRatio:  0.2,
		Compression: "lz4",
	}
}

func NewVPNClient(cfg *VPNConfig) (*VPNClient, error) {
	logCfg := zap.NewProductionConfig()
	logCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := logCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	appCfg := &config.Config{
		Client: config.ClientConfig{
			ServerAddr:     cfg.ServerAddr,
			ServerPort:     cfg.ServerPort,
			Username:       cfg.Username,
			Password:       cfg.Password,
			PrivateKeyPath: cfg.PrivateKeyPath,
			TUNName:        cfg.TUNName,
			TUNAddr:        cfg.TUNAddr,
			TUNNetmask:     cfg.TUNNetmask,
			MTU:            cfg.MTU,
		},
		Channels: config.ChannelsConfig{
			MinRead:    cfg.MinRead,
			MaxRead:    cfg.MaxRead,
			MinWrite:   cfg.MinWrite,
			MaxWrite:   cfg.MaxWrite,
			ReadRatio:  cfg.ReadRatio,
			WriteRatio: cfg.WriteRatio,
			HealthCheck: 5 * time.Second,
			Timeout:     30 * time.Second,
		},
		Security: config.SecurityConfig{
			Compression: cfg.Compression,
		},
	}

	var comp compress.Compressor
	switch cfg.Compression {
	case "lz4":
		comp, err = compress.NewLZ4Compressor(logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}
	default:
		comp = compress.NewNoopCompressor()
	}

	channelMgr := channel.NewManager(
		cfg.MinRead, cfg.MaxRead,
		cfg.MinWrite, cfg.MaxWrite,
		cfg.ReadRatio, cfg.WriteRatio,
		5*time.Second, 30*time.Second,
		logger,
	)

	return &VPNClient{
		config:     appCfg,
		channelMgr: channelMgr,
		compressor: comp,
		logger:     logger,
		status:     "disconnected",
	}, nil
}

func (v *VPNClient) SetOnStatus(callback func(string)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.onStatus = callback
}

func (v *VPNClient) SetOnError(callback func(string)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.onError = callback
}

func (v *VPNClient) SetOnStats(callback func(map[string]interface{})) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.onStats = callback
}

func (v *VPNClient) updateStatus(status string) {
	v.mu.Lock()
	v.status = status
	cb := v.onStatus
	v.mu.Unlock()
	if cb != nil {
		cb(status)
	}
}

func (v *VPNClient) reportError(err string) {
	v.mu.Lock()
	cb := v.onError
	v.mu.Unlock()
	if cb != nil {
		cb(err)
	}
}

func (v *VPNClient) reportStats(stats map[string]interface{}) {
	v.mu.Lock()
	cb := v.onStats
	v.mu.Unlock()
	if cb != nil {
		cb(stats)
	}
}

func (v *VPNClient) Connect() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.updateStatus("connecting")

	sshCfg, err := sshtransport.NewSSHClientConfig(&sshtransport.ClientConfig{
		ServerAddr:     v.config.Client.ServerAddr,
		Username:       v.config.Client.Username,
		Password:       v.config.Client.Password,
		PrivateKeyPath: v.config.Client.PrivateKeyPath,
	}, v.logger)
	if err != nil {
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	transport := sshtransport.NewTransport(
		v.config.Client.ServerAddr,
		sshCfg,
		v.logger,
	)

	if err := transport.Connect(); err != nil {
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	v.transport = transport

	handshake := sshtransport.NewClientHandshake(transport.GetConnection(), v.logger)
	handshake.SetChannelRatios(v.config.Channels.ReadRatio, v.config.Channels.WriteRatio)
	handshake.SetChannelLimits(uint32(v.config.Channels.MinRead), uint32(v.config.Channels.MaxRead))

	if err := handshake.DoClientHandshake(); err != nil {
		transport.Close()
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	v.handshake = handshake

	negotiator := sshtransport.NewChannelNegotiator(handshake, v.channelMgr, v.config.Channels.MinWrite, v.logger)
	if err := negotiator.NegotiateChannels(); err != nil {
		transport.Close()
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	v.balancer = balancer.NewBalancer(v.channelMgr, balancer.StrategyWeightedRoundRobin, v.logger)

	transport.StartKeepalive(15 * time.Second)

	v.updateStatus("connected")
	return nil
}

func (v *VPNClient) StartTunnel() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return fmt.Errorf("tunnel already running")
	}

	tunIface, err := tun.New(tun.Config{
		Name:    v.config.Client.TUNName,
		Addr:    v.config.Client.TUNAddr,
		Netmask: v.config.Client.TUNNetmask,
		MTU:     v.config.Client.MTU,
	}, v.logger)
	if err != nil {
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	v.tunIface = tunIface
	v.tunnel = tun.NewTunnel(tunIface, v.channelMgr, v.compressor, v.config.Client.MTU, v.logger)

	if err := v.tunnel.Start(); err != nil {
		v.tunIface.Close()
		v.updateStatus("error")
		v.reportError(err.Error())
		return err
	}

	v.running = true
	v.updateStatus("tunnel_active")

	go v.monitorLoop()

	return nil
}

func (v *VPNClient) monitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		v.mu.RLock()
		running := v.running
		transport := v.transport
		v.mu.RUnlock()

		if !running {
			return
		}

		if transport != nil && !transport.IsConnected() {
			v.updateStatus("reconnecting")
			v.reconnect()
		}

		readCount, writeCount := v.channelMgr.ChannelCount()
		v.reportStats(map[string]interface{}{
			"read_channels":  readCount,
			"write_channels": writeCount,
			"connected":      v.IsConnected(),
		})
	}
}

func (v *VPNClient) reconnect() {
	v.mu.Lock()

	if v.tunnel != nil {
		v.tunnel.Stop()
	}

	if v.transport != nil {
		v.transport.Close()
	}

	v.channelMgr.CloseAll()
	v.mu.Unlock()

	for i := 0; i < 5; i++ {
		v.logger.Info("attempting to reconnect", zap.Int("attempt", i+1))

		sshCfg, err := sshtransport.NewSSHClientConfig(&sshtransport.ClientConfig{
			ServerAddr:     v.config.Client.ServerAddr,
			Username:       v.config.Client.Username,
			Password:       v.config.Client.Password,
			PrivateKeyPath: v.config.Client.PrivateKeyPath,
		}, v.logger)
		if err != nil {
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		transport := sshtransport.NewTransport(
			v.config.Client.ServerAddr,
			sshCfg,
			v.logger,
		)

		if err := transport.Connect(); err != nil {
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		handshake := sshtransport.NewClientHandshake(transport.GetConnection(), v.logger)
		handshake.SetChannelRatios(v.config.Channels.ReadRatio, v.config.Channels.WriteRatio)
		handshake.SetChannelLimits(uint32(v.config.Channels.MinRead), uint32(v.config.Channels.MaxRead))

		if err := handshake.DoClientHandshake(); err != nil {
			transport.Close()
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		negotiator := sshtransport.NewChannelNegotiator(handshake, v.channelMgr, v.config.Channels.MinWrite, v.logger)
		if err := negotiator.NegotiateChannels(); err != nil {
			transport.Close()
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		transport.StartKeepalive(15 * time.Second)

		v.mu.Lock()
		v.transport = transport
		v.handshake = handshake
		v.balancer = balancer.NewBalancer(v.channelMgr, balancer.StrategyWeightedRoundRobin, v.logger)
		v.mu.Unlock()

		v.tunnel = tun.NewTunnel(v.tunIface, v.channelMgr, v.compressor, v.config.Client.MTU, v.logger)
		if err := v.tunnel.Start(); err != nil {
			v.logger.Error("failed to restart tunnel after reconnect", zap.Error(err))
			transport.Close()
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		v.updateStatus("connected")
		return
	}

	v.updateStatus("error")
	v.reportError("failed to reconnect")
}

func (v *VPNClient) Disconnect() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.running {
		return
	}

	v.running = false

	if v.tunnel != nil {
		v.tunnel.Stop()
	}

	if v.transport != nil {
		v.transport.Close()
	}

	if v.tunIface != nil {
		v.tunIface.Close()
	}

	v.updateStatus("disconnected")
}

func (v *VPNClient) GetStatus() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.status
}

func (v *VPNClient) IsConnected() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.transport != nil && v.transport.IsConnected()
}

func (v *VPNClient) GetChannelStats() map[string]interface{} {
	readCount, writeCount := v.channelMgr.ChannelCount()
	return map[string]interface{}{
		"read_channels":  readCount,
		"write_channels": writeCount,
	}
}
