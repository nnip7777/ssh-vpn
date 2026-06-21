//go:build linux

package tun

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"go.uber.org/zap"
)

const (
	tunDevice  = "/dev/net/tun"
	ifnameSize = 16
)

type ifReq struct {
	Name  [ifnameSize]byte
	Flags uint16
	_     [22]byte
}

const (
	IFF_TUN   = 0x0001
	IFF_NO_PI = 0x1000
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
	fd, err := syscall.Open(tunDevice, os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", tunDevice, err)
	}

	var req ifReq
	req.Flags = IFF_TUN | IFF_NO_PI
	copy(req.Name[:], cfg.Name)

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TUNSETIFF, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("failed to create TUN interface: %w", errno)
	}

	actualName := strings.TrimRight(string(req.Name[:]), "\x00")

	file := os.NewFile(uintptr(fd), tunDevice)

	tun := &Interface{
		file:    file,
		name:    actualName,
		addr:    cfg.Addr,
		netmask: cfg.Netmask,
		logger:  logger,
	}

	if err := tun.configure(cfg); err != nil {
		file.Close()
		return nil, err
	}

	logger.Info("TUN interface created",
		zap.String("name", actualName),
		zap.String("addr", cfg.Addr))

	return tun, nil
}

func (t *Interface) configure(cfg Config) error {
	sockfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer syscall.Close(sockfd)

	if err := t.setAddr(sockfd, cfg.Addr, cfg.Netmask); err != nil {
		return err
	}

	if err := t.setUp(sockfd); err != nil {
		return err
	}

	return nil
}

func (t *Interface) setAddr(sockfd int, addr, netmask string) error {
	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("invalid address: %s", addr)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("address is not IPv4: %s", addr)
	}

	type ifreqAddr struct {
		Name [ifnameSize]byte
		Addr [16]byte
	}

	var ifr ifreqAddr
	copy(ifr.Name[:], t.name)
	ifr.Addr[0] = byte(syscall.AF_INET)
	ifr.Addr[1] = byte(syscall.AF_INET >> 8)
	copy(ifr.Addr[4:8], ip4)

	// SIOCSIFADDR = 0x8916
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(sockfd), 0x8916, uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		return fmt.Errorf("failed to set address: %w", errno)
	}

	mask := net.IPMask(net.ParseIP(netmask).To4())
	if mask == nil {
		mask = net.CIDRMask(24, 32)
	}

	var ifrMask ifreqAddr
	copy(ifrMask.Name[:], t.name)
	ifrMask.Addr[0] = byte(syscall.AF_INET)
	ifrMask.Addr[1] = byte(syscall.AF_INET >> 8)
	copy(ifrMask.Addr[4:8], mask)

	// SIOCSIFNETMASK = 0x891c
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(sockfd), 0x891c, uintptr(unsafe.Pointer(&ifrMask)))
	if errno != 0 {
		return fmt.Errorf("failed to set netmask: %w", errno)
	}

	return nil
}

func (t *Interface) setUp(sockfd int) error {
	var req ifReq
	copy(req.Name[:], t.name)

	// SIOCGIFFLAGS = 0x8913
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(sockfd), 0x8913, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return fmt.Errorf("failed to get flags: %w", errno)
	}

	req.Flags |= syscall.IFF_UP | syscall.IFF_RUNNING

	// SIOCSIFFLAGS = 0x8914
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(sockfd), 0x8914, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return fmt.Errorf("failed to set flags: %w", errno)
	}

	return nil
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
	sockfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(sockfd)

	var req ifReq
	copy(req.Name[:], t.name)

	// SIOCSIFMTU = 0x8922
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(sockfd), 0x8922, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return fmt.Errorf("failed to set MTU: %w", errno)
	}

	return nil
}

var _ io.ReadWriteCloser = (*Interface)(nil)
