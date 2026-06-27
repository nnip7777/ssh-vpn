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
	"golang.org/x/sys/unix"
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
	Peer    string
	Netmask string
	MTU     int
}

func New(cfg Config, logger *zap.Logger) (*Interface, error) {
	fd, name, err := openUTUN()
	if err != nil {
		return nil, fmt.Errorf("failed to create UTUN: %w", err)
	}

	unix.SetNonblock(fd, true)
	unix.CloseOnExec(fd)
	file := os.NewFile(uintptr(fd), "")

	tun := &Interface{
		file:    file,
		name:    name,
		addr:    cfg.Addr,
		netmask: cfg.Netmask,
		logger:  logger,
	}

	if err := tun.configure(cfg); err != nil {
		file.Close()
		return nil, err
	}

	ip := net.ParseIP(cfg.Addr).To4()
	if ip != nil {
		ip[3] = 0
		subnet := fmt.Sprintf("%s/24", ip.String())
		exec.Command("route", "-n", "add", "-net", subnet, "-interface", name).Run()
	}

	logger.Info("TUN interface created",
		zap.String("name", name),
		zap.String("addr", cfg.Addr))

	return tun, nil
}

func openUTUN() (int, string, error) {
	for i := 0; i < 10; i++ {
		fd, name, err := tryUTUN(i)
		if err == nil {
			return fd, name, nil
		}
	}
	return -1, "", fmt.Errorf("no UTUN device available")
}

func tryUTUN(unit int) (int, string, error) {
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, 2)
	if err != nil {
		return -1, "", fmt.Errorf("socket: %w", err)
	}

	ctlInfo := &unix.CtlInfo{}
	copy(ctlInfo.Name[:], []byte("com.apple.net.utun_control"))
	if err := unix.IoctlCtlInfo(fd, ctlInfo); err != nil {
		unix.Close(fd)
		return -1, "", fmt.Errorf("IoctlCtlInfo: %w", err)
	}

	sc := &unix.SockaddrCtl{
		ID:   ctlInfo.Id,
		Unit: uint32(unit) + 1,
	}

	if err := unix.Connect(fd, sc); err != nil {
		unix.Close(fd)
		return -1, "", fmt.Errorf("Connect: %w", err)
	}

	name, err := unix.GetsockoptString(fd, 2, 2)
	if err != nil {
		unix.Close(fd)
		return -1, "", fmt.Errorf("GetSockoptString: %w", err)
	}

	return fd, name, nil
}

func (t *Interface) configure(cfg Config) error {
	commands := [][]string{
		{"ifconfig", t.name, cfg.Addr, cfg.Peer, "netmask", cfg.Netmask},
		{"ifconfig", t.name, "up"},
		{"ifconfig", t.name, "mtu", fmt.Sprintf("%d", cfg.MTU)},
	}
	for _, cmd := range commands {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("failed: %s: %s: %w", cmd, string(out), err)
		}
	}
	return nil
}

func (t *Interface) Read(p []byte) (int, error) {
	buf := make([]byte, len(p)+4)
	n, err := t.file.Read(buf)
	if n < 4 {
		return 0, err
	}
	copy(p, buf[4:n])
	return n - 4, nil
}

func (t *Interface) Write(p []byte) (int, error) {
	buf := make([]byte, len(p)+4)
	buf[0] = 0x00
	buf[1] = 0x00
	buf[2] = 0x00
	buf[3] = unix.AF_INET
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
	out, err := exec.Command("ifconfig", t.name, "mtu", fmt.Sprintf("%d", mtu)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("setmtu: %s: %w", string(out), err)
	}
	return nil
}

var _ io.ReadWriteCloser = (*Interface)(nil)
