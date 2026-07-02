package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type ServerConfig struct {
	HostKeyPath        string
	AuthorizedKeysPath string
	MaxAuthTries       int
	Password           string
}

type rateLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	maxFails int
	window   time.Duration
}

func newRateLimiter(maxFails int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		failures: make(map[string][]time.Time),
		maxFails: maxFails,
		window:   window,
	}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	times := r.failures[ip]
	filtered := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	r.failures[ip] = filtered

	return len(filtered) < r.maxFails
}

func (r *rateLimiter) recordFailure(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[ip] = append(r.failures[ip], time.Now())
}

func GenerateHostKey(path string) error {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	privBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	return pem.Encode(file, privBytes)
}

func LoadHostKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read host key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	return signer, nil
}

func LoadAuthorizedKeys(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read authorized keys: %w", err)
	}

	keys := make(map[string]bool)
	for len(data) > 0 {
		key, comment, _, rest, err := ssh.ParseAuthorizedKey(data)
		if err != nil {
			data = rest
			continue
		}
		fp := ssh.FingerprintSHA256(key)
		keys[fp] = true
		_ = comment
		data = rest
	}

	return keys, nil
}

func NewSSHServerConfig(cfg *ServerConfig, logger *zap.Logger) (*ssh.ServerConfig, error) {
	hostKey, err := LoadHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ServerConfig{
		MaxAuthTries:  cfg.MaxAuthTries,
		ServerVersion: "SSH-2.0-OpenSSH_8.9",
	}

	sshConfig.AddHostKey(hostKey)

	var authorizedKeys map[string]bool
	if cfg.AuthorizedKeysPath != "" {
		authorizedKeys, err = LoadAuthorizedKeys(cfg.AuthorizedKeysPath)
		if err != nil {
			logger.Warn("failed to load authorized keys", zap.Error(err))
		}
	}

	limiter := newRateLimiter(10, 5*time.Minute)

	sshConfig.PublicKeyCallback = func(conn ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
		ip := conn.RemoteAddr().String()
		if !limiter.allow(ip) {
			logger.Warn("rate limited", zap.String("from", ip))
			return nil, fmt.Errorf("rate limited")
		}
		if authorizedKeys != nil {
			fp := ssh.FingerprintSHA256(pubKey)
			if authorizedKeys[fp] {
				return &ssh.Permissions{
					Extensions: map[string]string{
						"pubkey-fp": fp,
					},
				}, nil
			}
		}
		limiter.recordFailure(ip)
		return nil, fmt.Errorf("unauthorized")
	}

	sshConfig.PasswordCallback = func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		ip := conn.RemoteAddr().String()
		if !limiter.allow(ip) {
			logger.Warn("rate limited", zap.String("from", ip), zap.String("user", conn.User()))
			return nil, fmt.Errorf("rate limited")
		}
		if cfg.Password == "" {
			logger.Warn("password auth disabled, no password configured",
				zap.String("user", conn.User()), zap.String("from", ip))
			return nil, fmt.Errorf("password auth disabled")
		}
		if subtle.ConstantTimeCompare(password, []byte(cfg.Password)) != 1 {
			limiter.recordFailure(ip)
			logger.Warn("wrong password",
				zap.String("user", conn.User()), zap.String("from", ip))
			return nil, fmt.Errorf("authentication failed")
		}
		logger.Info("password auth success",
			zap.String("user", conn.User()), zap.String("from", ip))
		return &ssh.Permissions{}, nil
	}

	sshConfig.KeyboardInteractiveCallback = func(conn ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		return nil, fmt.Errorf("keyboard-interactive not supported")
	}

	return sshConfig, nil
}

type ClientConfig struct {
	ServerAddr     string
	Username       string
	Password       string
	PrivateKeyPath string
}

func NewSSHClientConfig(cfg *ClientConfig, logger *zap.Logger) (*ssh.ClientConfig, error) {
	sshConfig := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if cfg.Password != "" {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(cfg.Password))
	}

	if cfg.PrivateKeyPath != "" {
		key, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
	}

	if len(sshConfig.Auth) == 0 {
		return nil, fmt.Errorf("no authentication method provided")
	}

	return sshConfig, nil
}

var _ net.Addr = (*sshServerAddr)(nil)

type sshServerAddr struct {
	addr string
}

func (a *sshServerAddr) Network() string { return "tcp" }
func (a *sshServerAddr) String() string  { return a.addr }
