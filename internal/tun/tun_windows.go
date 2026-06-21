//go:build windows

package tun

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"

	"go.uber.org/zap"
)

type Interface struct {
	file    *os.File
	name    string
	addr    string
	netmask string
	logger  *zap.Logger
	mu      sync.RWMutex
}

type Config struct {
	Name    string
	Addr    string
	Netmask string
	MTU     int
}

func New(cfg Config, logger *zap.Logger) (*Interface, error) {
	tapName := fmt.Sprintf("ssh_vpn_%s", cfg.Name)

	cmd := exec.Command("netsh", "interface", "show", "interface")
	if err := cmd.Run(); err != nil {
		logger.Warn("netsh not available, TUN not supported on this Windows version")
	}

	file, err := os.OpenFile("\\\\.\\Global\\ssh_vpn.tap", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open TAP device: %w", err)
	}

	tun := &Interface{
		file:    file,
		name:    tapName,
		addr:    cfg.Addr,
		netmask: cfg.Netmask,
		logger:  logger,
	}

	logger.Info("TUN interface created",
		zap.String("name", tapName),
		zap.String("addr", cfg.Addr))

	return tun, nil
}

func (t *Interface) Read(p []byte) (int, error) {
	return t.file.Read(p)
}

func (t *Interface) Write(p []byte) (int, error) {
	return t.file.Write(p)
}

func (t *Interface) Close() error {
	return t.file.Close()
}

func (t *Interface) Name() string {
	return t.name
}

func (t *Interface) IP() net.IP {
	return net.ParseIP(t.addr)
}

func (t *Interface) SetMTU(mtu int) error {
	cmd := exec.Command("netsh", "interface", "ipv4", "set", "subinterface", t.name, fmt.Sprintf("mtu=%d", mtu))
	return cmd.Run()
}

var _ io.ReadWriteCloser = (*Interface)(nil)
