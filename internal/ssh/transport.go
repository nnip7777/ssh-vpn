package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/tun"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type Transport struct {
	config     *ssh.ClientConfig
	serverAddr string
	conn       *ssh.Client
	mu         sync.RWMutex
	logger     *zap.Logger
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

	conn, err := ssh.Dial("tcp", t.serverAddr, t.config)
	if err != nil {
		return fmt.Errorf("failed to dial SSH server: %w", err)
	}

	t.conn = conn
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

	t.logger.Info("opened SSH channel",
		zap.String("type", channelType))

	return ch, reqs, nil
}

func (t *Transport) Close() error {
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
}

type ClientSession struct {
	conn     *ssh.ServerConn
	channels map[uint16]ssh.Channel
	mu       sync.RWMutex
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
	}, nil
}

func (s *Server) Accept() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.logger.Error("failed to accept connection", zap.Error(err))
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(netConn net.Conn) {
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
	}

	s.mu.Lock()
	s.clients[sshConn.RemoteAddr().String()] = client
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, sshConn.RemoteAddr().String())
		s.mu.Unlock()
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
	client.mu.Lock()
	client.channels[id] = ch
	client.mu.Unlock()

	s.logger.Info("new channel opened",
		zap.String("client", client.conn.RemoteAddr().String()),
		zap.Uint16("id", id),
		zap.String("type", newChan.ChannelType()))

	go func() {
		for req := range reqs {
			s.handleRequest(client, id, req)
		}
	}()

	go s.handleChannelData(client, id, ch)
}

func (s *Server) handleRequest(client *ClientSession, channelID uint16, req *ssh.Request) {
	s.logger.Debug("handling request",
		zap.String("client", client.conn.RemoteAddr().String()),
		zap.Uint16("channel", channelID),
		zap.String("type", req.Type))

	if req.WantReply {
		req.Reply(true, nil)
	}
}

func (s *Server) handleChannelData(client *ClientSession, channelID uint16, ch ssh.Channel) {
	defer func() {
		client.mu.Lock()
		delete(client.channels, channelID)
		client.mu.Unlock()
		ch.Close()
	}()

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
			if _, err := s.tunIface.Write(buf[:n]); err != nil {
				s.logger.Error("failed to write to TUN",
					zap.Error(err),
					zap.Uint16("channel", channelID))
			}
		}
	}
}

func (s *Server) Close() error {
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
