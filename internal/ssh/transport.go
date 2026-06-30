package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/tun"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

var serverBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 1600)
		return &b
	},
}

func serverGetBuf() *[]byte {
	return serverBufPool.Get().(*[]byte)
}

func serverPutBuf(b *[]byte) {
	if cap(*b) <= 1600 {
		*b = (*b)[:0]
		serverBufPool.Put(b)
	}
}

type Transport struct {
	config        *ssh.ClientConfig
	serverAddr    string
	conn          *ssh.Client
	mu            sync.RWMutex
	logger        *zap.Logger
	keepaliveStop chan struct{}
}

func NewTransport(serverAddr string, config *ssh.ClientConfig, logger *zap.Logger) *Transport {
	return &Transport{
		config:     config,
		serverAddr: serverAddr,
		logger:     logger,
	}
}

func (t *Transport) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tcpConn, err := net.Dial("tcp", t.serverAddr)
	if err != nil {
		return fmt.Errorf("failed to dial TCP: %w", err)
	}

	if tc, ok := tcpConn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(30 * time.Second)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, t.serverAddr, t.config)
	if err != nil {
		tcpConn.Close()
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	t.conn = ssh.NewClient(sshConn, chans, reqs)
	t.logger.Info("connected to SSH server", zap.String("addr", t.serverAddr))
	return nil
}

func (t *Transport) OpenChannel(channelType string) (ssh.Channel, <-chan *ssh.Request, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return nil, nil, fmt.Errorf("not connected")
	}

	ch, reqs, err := t.conn.OpenChannel(channelType, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open channel: %w", err)
	}

	t.logger.Info("opened SSH channel", zap.String("type", channelType))

	return ch, reqs, nil
}

func (t *Transport) Close() error {
	t.StopKeepalive()

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}

func (t *Transport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.conn != nil
}

func (t *Transport) StartKeepalive(interval time.Duration) {
	t.StopKeepalive()
	t.keepaliveStop = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-t.keepaliveStop:
				return
			case <-ticker.C:
				t.mu.RLock()
				conn := t.conn
				t.mu.RUnlock()
				if conn == nil {
					return
				}
				ok, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil || !ok {
					t.logger.Warn("SSH keepalive failed, connection may be dead",
						zap.Error(err), zap.Bool("reply", ok))
					t.mu.Lock()
					if t.conn != nil {
						t.conn.Close()
						t.conn = nil
					}
					t.mu.Unlock()
					return
				}
			}
		}
	}()
}

func (t *Transport) StopKeepalive() {
	if t.keepaliveStop != nil {
		close(t.keepaliveStop)
		t.keepaliveStop = nil
	}
}

func (t *Transport) GetConnection() *ssh.Client {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.conn
}

type Server struct {
	addr       string
	config     *ssh.ServerConfig
	listener   net.Listener
	manager    *channel.Manager
	tunIface   *tun.Interface
	clients    map[string]*ClientSession
	mu         sync.RWMutex
	logger     *zap.Logger
	toTUN      chan []byte
	fromTUN    chan []byte
	stopCh     chan struct{}
	running    bool
}

type ClientSession struct {
	conn       *ssh.ServerConn
	channels   map[uint16]ssh.Channel
	types      map[uint16]string
	writeCh    chan []byte
	stopCh     chan struct{}
	readChs    []ssh.Channel
	channelsVer int
	isVPN      bool
	mu         sync.RWMutex
}

func NewServer(addr string, config *ssh.ServerConfig, manager *channel.Manager, tunIface *tun.Interface, logger *zap.Logger) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	return &Server{
		addr:     addr,
		config:   config,
		listener: listener,
		manager:  manager,
		tunIface: tunIface,
		clients:  make(map[string]*ClientSession),
		logger:   logger,
		toTUN:    make(chan []byte, 8192),
		fromTUN:  make(chan []byte, 8192),
		stopCh:   make(chan struct{}),
	}, nil
}

func (s *Server) Start() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	go s.readFromTUN()
	go s.writeToTUN()

	s.logger.Info("server tunnel started")
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopCh)
	s.logger.Info("server tunnel stopped")
}

func (s *Server) readFromTUN() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		bufp := serverGetBuf()
		buf := (*bufp)[:1500]
		n, err := s.tunIface.Read(buf)
		if err != nil {
			serverPutBuf(bufp)
			if err != io.EOF {
				s.logger.Error("TUN read error", zap.Error(err))
			}
			continue
		}

		if n == 0 {
			serverPutBuf(bufp)
			continue
		}

		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		serverPutBuf(bufp)
		select {
		case s.fromTUN <- pkt:
		default:
		}
	}
}

func (s *Server) writeToTUN() {
	for {
		select {
		case <-s.stopCh:
			return
		case pkt := <-s.toTUN:
			if _, werr := s.tunIface.Write(pkt); werr != nil {
				s.logger.Error("TUN write error", zap.Error(werr))
			}
		}
	}
}

func (s *Server) BroadcastToClients() {
	for {
		select {
		case <-s.stopCh:
			return
		case pkt := <-s.fromTUN:
			s.mu.RLock()
			for _, client := range s.clients {
				select {
				case client.writeCh <- pkt:
				default:
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *Server) clientWriter(client *ClientSession) {
	defer close(client.stopCh)
	var rrIndex uint64
	var lastVer int
	for {
		select {
		case <-s.stopCh:
			return
		case data, ok := <-client.writeCh:
			if !ok {
				return
			}

			client.mu.RLock()
			ver := len(client.channels)
			if ver != lastVer {
				client.readChs = client.readChs[:0]
				for id, ch := range client.channels {
					if client.types[id] == "vpn-read" {
						client.readChs = append(client.readChs, ch)
					}
				}
				lastVer = ver
			}
			readChs := client.readChs
			client.mu.RUnlock()

			if len(readChs) > 0 {
				idx := atomic.AddUint64(&rrIndex, 1) % uint64(len(readChs))
				readChs[idx].Write(data)
			}
		}
	}
}

func (s *Server) Accept() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.listener == nil
			s.mu.RUnlock()
			if closed {
				return nil
			}
			s.logger.Error("failed to accept connection", zap.Error(err))
			return err
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(netConn net.Conn) {
	if tc, ok := netConn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}

	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, s.config)
	if err != nil {
		s.logger.Error("failed to establish SSH connection", zap.Error(err))
		netConn.Close()
		return
	}

	defer sshConn.Close()

	client := &ClientSession{
		conn:     sshConn,
		channels: make(map[uint16]ssh.Channel),
		types:    make(map[uint16]string),
		writeCh:  make(chan []byte, 4096),
		stopCh:   make(chan struct{}),
		isVPN:    sshConn.User() == "vpnuser",
	}

	s.mu.Lock()
	s.clients[sshConn.RemoteAddr().String()] = client
	s.mu.Unlock()

	go s.clientWriter(client)

	defer func() {
		s.mu.Lock()
		delete(s.clients, sshConn.RemoteAddr().String())
		s.mu.Unlock()
		close(client.writeCh)
	}()

	s.logger.Info("client connected",
		zap.String("addr", sshConn.RemoteAddr().String()),
		zap.String("user", sshConn.User()))

	go ssh.DiscardRequests(reqs)

	handshake := NewServerHandshake(sshConn, s.logger)
	if err := handshake.DoServerHandshake(); err != nil {
		s.logger.Error("handshake failed", zap.Error(err))
		return
	}

	for newChan := range chans {
		s.handleChannel(client, newChan)
	}
}

func (s *Server) handleChannel(client *ClientSession, newChan ssh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		s.logger.Error("failed to accept channel", zap.Error(err))
		return
	}

	id := uint16(len(client.channels) + 1)
	channelType := newChan.ChannelType()
	client.mu.Lock()
	client.channels[id] = ch
	client.types[id] = channelType
	client.readChs = nil
	client.mu.Unlock()

	s.logger.Info("new channel opened",
		zap.String("client", client.conn.RemoteAddr().String()),
		zap.Uint16("id", id),
		zap.String("type", channelType),
		zap.Bool("vpn", client.isVPN))

	if !client.isVPN {
		go s.handleNormalSSH(ch, reqs)
		return
	}

	go func() {
		for req := range reqs {
			s.handleRequest(client, id, req)
		}
	}()

	if channelType == "vpn-write" {
		go s.handleChannelData(client, id, ch)
	}
}

func (s *Server) handleRequest(client *ClientSession, channelID uint16, req *ssh.Request) {
	if req.WantReply {
		req.Reply(true, nil)
	}
}

func (s *Server) handleNormalSSH(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "shell", "exec":
			if req.WantReply {
				ch.Write([]byte("Permission denied.\r\n"))
				req.Reply(false, nil)
			}
		case "keepalive@openssh.com":
			if req.WantReply {
				req.Reply(true, nil)
			}
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) handleChannelData(client *ClientSession, channelID uint16, ch ssh.Channel) {
	defer func() {
		client.mu.Lock()
		delete(client.channels, channelID)
		client.readChs = nil
		client.mu.Unlock()
		ch.Close()
	}()

	s.logger.Info("channel data handler started", zap.Uint16("channel", channelID))

	buf := make([]byte, 32*1024)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logger.Error("channel read error",
					zap.Error(err),
					zap.Uint16("channel", channelID))
			}
			return
		}

		if n > 0 {
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			select {
			case s.toTUN <- pkt:
			default:
			}
		}
	}
}

func (s *Server) Close() error {
	s.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()

	for addr, client := range s.clients {
		client.conn.Close()
		delete(s.clients, addr)
	}

	return s.listener.Close()
}

func (s *Server) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func (s *Server) WaitForShutdown(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("shutdown timeout")
		default:
			if s.GetClientCount() == 0 {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
