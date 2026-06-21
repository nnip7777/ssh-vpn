//go:build darwin

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
	file, err := os.OpenFile("/dev/tun0", os.O_RDWR, 0)
	if err != nil {
		name, err2 := createUTUN(cfg.Name)
		if err2 != nil {
			return nil, fmt.Errorf("failed to create TUN: %w (utun: %w)", err, err2)
		}
		cfg.Name = name
		file, err = openUTUN(name)
		if err != nil {
			return nil, fmt.Errorf("failed to open UTUN: %w", err)
		}
	}

	tun := &Interface{
		file:    file,
		name:    cfg.Name,
		addr:    cfg.Addr,
		netmask: cfg.Netmask,
		logger:  logger,
	}

	if err := tun.configure(cfg); err != nil {
		file.Close()
		return nil, err
	}

	logger.Info("TUN interface created",
		zap.String("name", cfg.Name),
		zap.String("addr", cfg.Addr))

	return tun, nil
}

func createUTUN(name string) (string, error) {
	for i := 0; i < 10; i++ {
		ifname := fmt.Sprintf("utun%d", i)
		cmd := exec.Command("ifconfig", ifname, "create")
		if err := cmd.Run(); err == nil {
			return ifname, nil
		}
	}
	return "", fmt.Errorf("failed to create UTUN interface")
}

func openUTUN(name string) (*os.File, error) {
	return os.OpenFile("/dev/"+name, os.O_RDWR, 0)
}

func (t *Interface) configure(cfg Config) error {
	commands := [][]string{
		{"ifconfig", t.name, cfg.Addr, cfg.Addr, "netmask", cfg.Netmask, "up"},
		{"ifconfig", t.name, "mtu", fmt.Sprintf("%d", cfg.MTU)},
	}

	for _, cmd := range commands {
		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			return fmt.Errorf("failed to execute %s: %w", cmd[0], err)
		}
	}

	return nil
}

func (t *Interface) Read(p []byte) (int, error) {
	buf := make([]byte, len(p)+4)
	n, err := t.file.Read(buf)
	if err != nil {
		return 0, err
	}
	copy(p, buf[4:])
	return n - 4, nil
}

func (t *Interface) Write(p []byte) (int, error) {
	buf := make([]byte, len(p)+4)
	copy(buf[4:], p)
	return t.file.Write(buf)
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
	cmd := exec.Command("ifconfig", t.name, "mtu", fmt.Sprintf("%d", mtu))
	return cmd.Run()
}

func (t *Interface) runRoute(args ...string) error {
	cmd := exec.Command("route", args...)
	return cmd.Run()
}

var _ io.ReadWriteCloser = (*Interface)(nil)

func ParseIPv4Header(data []byte) (headerLen int, protocol byte, src, dst net.IP) {
	if len(data) < 20 {
		return 0, 0, nil, nil
	}
	headerLen = int(data[0]&0x0F) * 4
	protocol = data[9]
	src = net.IP(data[12:16])
	dst = net.IP(data[16:20])
	return
}
