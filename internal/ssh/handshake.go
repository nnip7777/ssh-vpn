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
	h.state = StateReady
	h.logger.Info("handshake complete")

	return nil
}

func (h *Handshake) DoServerHandshake() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("starting server handshake")
	h.state = StateReady
	h.logger.Info("handshake complete (server)")

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
	minWrite  int

	fightMode       bool
	recentDeaths    int
	deathMu         sync.Mutex
	deathWindowStart time.Time
	refreshInterval time.Duration
}

func NewChannelNegotiator(handshake *Handshake, manager *channel.Manager, minWrite int, logger *zap.Logger) *ChannelNegotiator {
	return &ChannelNegotiator{
		handshake:       handshake,
		manager:         manager,
		logger:          logger,
		minWrite:        minWrite,
		refreshInterval: 30 * time.Second,
		deathWindowStart: time.Now(),
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
	maxRead := int(cn.handshake.maxChannels)
	minRead := int(cn.handshake.minChannels)
	cn.handshake.mu.RUnlock()

	minWrite := cn.minWrite
	if minWrite < 2 {
		minWrite = 2
	}
	maxWrite := maxRead / 2
	if maxWrite < minWrite {
		maxWrite = minWrite
	}

	if writeCount < minWrite {
		cn.RecordDeath()
		cn.logger.Warn("write channels below minimum, creating replacement",
			zap.Int("current", writeCount),
			zap.Int("min", minWrite))
		for writeCount < minWrite {
			if err := cn.createChannel(channel.ChannelWrite); err != nil {
				cn.logger.Error("failed to create write channel", zap.Error(err))
				break
			}
			writeCount++
		}
	}

	if readCount < minRead {
		cn.RecordDeath()
		cn.logger.Warn("read channels below minimum, creating replacement",
			zap.Int("current", readCount),
			zap.Int("min", minRead))
		for readCount < minRead {
			if err := cn.createChannel(channel.ChannelRead); err != nil {
				cn.logger.Error("failed to create read channel", zap.Error(err))
				break
			}
			readCount++
		}
	}

	if totalBytesRead+totalBytesWrite == 0 {
		return
	}

	currentRatio := float64(totalBytesRead) / float64(totalBytesRead+totalBytesWrite)

	if currentRatio > targetRatio+0.1 && readCount < maxRead {
		cn.createChannel(channel.ChannelRead)
	}

	if currentRatio < targetRatio-0.1 && writeCount < maxWrite {
		cn.createChannel(channel.ChannelWrite)
	}
}

func (cn *ChannelNegotiator) RecordDeath() {
	cn.deathMu.Lock()
	defer cn.deathMu.Unlock()

	now := time.Now()
	if now.Sub(cn.deathWindowStart) > 30*time.Second {
		cn.recentDeaths = 0
		cn.deathWindowStart = now
	}
	cn.recentDeaths++
}

func (cn *ChannelNegotiator) maybeAdjustMode() {
	cn.deathMu.Lock()
	defer cn.deathMu.Unlock()

	if cn.recentDeaths > 2 && !cn.fightMode {
		cn.fightMode = true
		cn.refreshInterval = 5 * time.Second
		cn.logger.Warn("entered fight mode",
			zap.Int("deaths_in_30s", cn.recentDeaths))
	}

	if cn.recentDeaths == 0 && cn.fightMode {
		cn.fightMode = false
		cn.refreshInterval = 30 * time.Second
		cn.logger.Info("exited fight mode")
	}
}

func (cn *ChannelNegotiator) ProactiveRefresh(stopCh <-chan struct{}) {
	currentInterval := cn.refreshInterval
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			cn.maybeAdjustMode()

			cn.deathMu.Lock()
			newInterval := cn.refreshInterval
			cn.deathMu.Unlock()

			if newInterval != currentInterval {
				currentInterval = newInterval
				ticker.Reset(currentInterval)
			}

			cn.refreshOldestChannel()
		}
	}
}

func (cn *ChannelNegotiator) refreshOldestChannel() {
	cn.handshake.mu.RLock()
	maxRead := int(cn.handshake.maxChannels)
	cn.handshake.mu.RUnlock()

	maxWrite := maxRead / 2
	if maxWrite < cn.minWrite {
		maxWrite = cn.minWrite
	}

	readCount, writeCount := cn.manager.ChannelCount()

	if readCount >= maxRead && writeCount >= maxWrite {
		return
	}

	if readCount < maxRead {
		if err := cn.createChannel(channel.ChannelRead); err != nil {
			cn.logger.Debug("proactive refresh: failed to create read channel", zap.Error(err))
			return
		}
		cn.logger.Debug("proactive refresh: created read channel",
			zap.Int("total_read", readCount+1))
	}

	if writeCount < maxWrite {
		if err := cn.createChannel(channel.ChannelWrite); err != nil {
			cn.logger.Debug("proactive refresh: failed to create write channel", zap.Error(err))
			return
		}
		cn.logger.Debug("proactive refresh: created write channel",
			zap.Int("total_write", writeCount+1))
	}
}

func (cn *ChannelNegotiator) Close() error {
	cn.manager.CloseAll()
	return nil
}
