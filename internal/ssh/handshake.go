package ssh

import (
	"fmt"
	"sync"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

const (
	ChannelTypeVPN     = "vpn-data"
	ChannelTypeControl = "vpn-control"
	ChannelTypeRead    = "vpn-read"
	ChannelTypeWrite   = "vpn-write"
)

type HandshakeState int

const (
	StateInit HandshakeState = iota
	StateVersion
	StateAuth
	StateChannels
	StateReady
	StateFailed
)

type Handshake struct {
	conn        *ssh.Client
	serverConn  *ssh.ServerConn
	state       HandshakeState
	clientID    [16]byte
	version     uint32
	readRatio   float64
	writeRatio  float64
	minChannels uint32
	maxChannels uint32
	mu          sync.RWMutex
	logger      *zap.Logger
}

func NewClientHandshake(conn *ssh.Client, logger *zap.Logger) *Handshake {
	h := &Handshake{
		conn:   conn,
		state:  StateInit,
		logger: logger,
	}
	copy(h.clientID[:], generateClientID())
	return h
}

func NewServerHandshake(conn *ssh.ServerConn, logger *zap.Logger) *Handshake {
	h := &Handshake{
		serverConn: conn,
		state:      StateInit,
		logger:     logger,
	}
	copy(h.clientID[:], generateClientID())
	return h
}

func generateClientID() []byte {
	id := make([]byte, 16)
	for i := range id {
		id[i] = byte(time.Now().UnixNano() & 0xFF)
		time.Sleep(time.Nanosecond)
	}
	return id
}

func (h *Handshake) DoClientHandshake() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("starting client handshake")
	h.state = StateVersion

	if err := h.sendVersion(); err != nil {
		h.state = StateFailed
		return fmt.Errorf("failed to send version: %w", err)
	}

	if err := h.recvVersion(); err != nil {
		h.state = StateFailed
		return fmt.Errorf("failed to receive version: %w", err)
	}

	h.state = StateAuth
	h.logger.Info("handshake version exchange complete")

	ch, reqs, err := h.conn.OpenChannel(ChannelTypeControl, nil)
	if err != nil {
		h.state = StateFailed
		return fmt.Errorf("failed to open control channel: %w", err)
	}
	defer ch.Close()

	go h.handleRequests(reqs)

	h.state = StateChannels
	h.logger.Info("control channel established")

	h.state = StateReady
	h.logger.Info("handshake complete")

	return nil
}

func (h *Handshake) DoServerHandshake() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("starting server handshake")
	h.state = StateVersion

	if err := h.recvVersion(); err != nil {
		h.state = StateFailed
		return fmt.Errorf("failed to receive version: %w", err)
	}

	if err := h.sendVersion(); err != nil {
		h.state = StateFailed
		return fmt.Errorf("failed to send version: %w", err)
	}

	h.state = StateReady
	h.logger.Info("handshake complete")

	return nil
}

func (h *Handshake) sendVersion() error {
	if h.conn == nil {
		return fmt.Errorf("not connected")
	}

	ch, _, err := h.conn.OpenChannel(ChannelTypeControl, nil)
	if err != nil {
		return err
	}
	defer ch.Close()

	buf := make([]byte, 48)
	buf[0] = 'S'
	buf[1] = 'S'
	buf[2] = 'H'
	buf[3] = '-'
	buf[4] = 'V'
	buf[5] = 'P'
	buf[6] = 'N'
	buf[7] = '-'
	buf[8] = '1'
	buf[9] = '.'
	buf[10] = '0'

	copy(buf[11:27], h.clientID[:])

	_, err = ch.Write(buf)
	return err
}

func (h *Handshake) recvVersion() error {
	if h.conn == nil {
		return fmt.Errorf("not connected")
	}

	ch, reqs, err := h.conn.OpenChannel(ChannelTypeControl, nil)
	if err != nil {
		return err
	}
	defer ch.Close()

	go h.handleRequests(reqs)

	buf := make([]byte, 48)
	_, err = ch.Read(buf)
	if err != nil {
		return err
	}

	h.version = 1

	h.logger.Info("received handshake",
		zap.Uint32("version", h.version))

	return nil
}

func (h *Handshake) handleRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		if req.Type == "keepalive@openssh.com" {
			if req.WantReply {
				req.Reply(true, nil)
			}
		} else {
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (h *Handshake) SetChannelRatios(read, write float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readRatio = read
	h.writeRatio = write
}

func (h *Handshake) SetChannelLimits(min, max uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.minChannels = min
	h.maxChannels = max
}

func (h *Handshake) GetState() HandshakeState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state
}

type ChannelNegotiator struct {
	handshake *Handshake
	manager   *channel.Manager
	logger    *zap.Logger
}

func NewChannelNegotiator(handshake *Handshake, manager *channel.Manager, logger *zap.Logger) *ChannelNegotiator {
	return &ChannelNegotiator{
		handshake: handshake,
		manager:   manager,
		logger:    logger,
	}
}

func (cn *ChannelNegotiator) NegotiateChannels() error {
	cn.handshake.mu.RLock()
	minRead := int(cn.handshake.minChannels)
	cn.handshake.mu.RUnlock()

	cn.logger.Info("negotiating channels",
		zap.Int("min_read", minRead))

	for i := 0; i < minRead; i++ {
		if err := cn.createChannel(channel.ChannelRead); err != nil {
			cn.logger.Warn("failed to create read channel", zap.Error(err))
		}
	}

	for i := 0; i < minRead/2; i++ {
		if err := cn.createChannel(channel.ChannelWrite); err != nil {
			cn.logger.Warn("failed to create write channel", zap.Error(err))
		}
	}

	return nil
}

func (cn *ChannelNegotiator) createChannel(channelType channel.ChannelType) error {
	var channelTypeStr string
	switch channelType {
	case channel.ChannelRead:
		channelTypeStr = ChannelTypeRead
	case channel.ChannelWrite:
		channelTypeStr = ChannelTypeWrite
	case channel.ChannelControl:
		channelTypeStr = ChannelTypeControl
	default:
		return fmt.Errorf("unknown channel type: %d", channelType)
	}

	ch, reqs, err := cn.handshake.conn.OpenChannel(channelTypeStr, nil)
	if err != nil {
		return fmt.Errorf("failed to open %s channel: %w", channelTypeStr, err)
	}

	go cn.handleChannelRequests(ch, reqs)

	readCount, writeCount := cn.manager.ChannelCount()
	id := uint16(readCount + writeCount)
	vpnChannel := channel.NewChannel(id, channelType, ch, ch, cn.logger)
	cn.manager.AddChannel(vpnChannel)

	cn.logger.Info("created channel",
		zap.Uint16("id", id),
		zap.String("type", channelTypeStr))

	return nil
}

func (cn *ChannelNegotiator) handleChannelRequests(ch ssh.Channel, reqs <-chan *ssh.Request) {
	for req := range reqs {
		if req.Type == "keepalive@openssh.com" {
			if req.WantReply {
				req.Reply(true, nil)
			}
		} else {
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (cn *ChannelNegotiator) MonitorChannels(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		cn.checkAndAdjustChannels()
	}
}

func (cn *ChannelNegotiator) checkAndAdjustChannels() {
	readCount, writeCount := cn.manager.ChannelCount()
	stats := cn.manager.GetStats()

	var totalBytesRead, totalBytesWrite uint64
	for _, stat := range stats {
		totalBytesRead += stat.BytesRecv
		totalBytesWrite += stat.BytesSent
	}

	cn.handshake.mu.RLock()
	targetRatio := cn.handshake.readRatio
	maxChannels := int(cn.handshake.maxChannels)
	cn.handshake.mu.RUnlock()

	if totalBytesRead+totalBytesWrite == 0 {
		return
	}

	currentRatio := float64(totalBytesRead) / float64(totalBytesRead+totalBytesWrite)

	cn.logger.Debug("channel stats",
		zap.Int("read_channels", readCount),
		zap.Int("write_channels", writeCount),
		zap.Float64("current_ratio", currentRatio),
		zap.Float64("target_ratio", targetRatio))

	if currentRatio > targetRatio+0.1 && readCount < maxChannels {
		cn.createChannel(channel.ChannelRead)
	}

	if currentRatio < targetRatio-0.1 && writeCount < maxChannels {
		cn.createChannel(channel.ChannelWrite)
	}
}

func (cn *ChannelNegotiator) Close() error {
	cn.manager.CloseAll()
	return nil
}
