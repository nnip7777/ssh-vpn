package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Client   ClientConfig   `yaml:"client"`
	Channels ChannelsConfig `yaml:"channels"`
	Security SecurityConfig `yaml:"security"`
	Monitor  MonitorConfig  `yaml:"monitor"`
}

type ServerConfig struct {
	ListenAddr    string        `yaml:"listen_addr"`
	ListenPort    int           `yaml:"listen_port"`
	ExtraPorts    []int         `yaml:"extra_ports"`
	MaxClients    int           `yaml:"max_clients"`
	Password      string        `yaml:"password"`
	TUNName       string        `yaml:"tun_name"`
	TUNAddr       string        `yaml:"tun_addr"`
	TUNNetmask    string        `yaml:"tun_netmask"`
	MTU           int           `yaml:"mtu"`
}

type ClientConfig struct {
	ServerAddr     string        `yaml:"server_addr"`
	ServerPort     int           `yaml:"server_port"`
	ExtraPorts     []int         `yaml:"extra_ports"`
	Username       string        `yaml:"username"`
	Password       string        `yaml:"password"`
	PrivateKeyPath string        `yaml:"private_key_path"`
	TUNName        string        `yaml:"tun_name"`
	TUNAddr        string        `yaml:"tun_addr"`
	TUNNetmask     string        `yaml:"tun_netmask"`
	MTU            int           `yaml:"mtu"`
	AutoConnect    bool          `yaml:"auto_connect"`
}

type ChannelsConfig struct {
	MinRead     int           `yaml:"min_read"`
	MaxRead     int           `yaml:"max_read"`
	MinWrite    int           `yaml:"min_write"`
	MaxWrite    int           `yaml:"max_write"`
	ReadRatio   float64       `yaml:"read_ratio"`
	WriteRatio  float64       `yaml:"write_ratio"`
	HealthCheck time.Duration `yaml:"health_check"`
	Timeout     time.Duration `yaml:"timeout"`
}

type SecurityConfig struct {
	Encryption  string `yaml:"encryption"`
	Compression string `yaml:"compression"`
}

type MonitorConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listen_addr"`
	ListenPort int    `yaml:"listen_port"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr: "0.0.0.0",
			ListenPort: 2222,
			MaxClients: 100,
			TUNName:    "ssh-vpn0",
			TUNAddr:    "10.8.0.1",
			TUNNetmask: "255.255.255.0",
			MTU:        1400,
		},
		Client: ClientConfig{
			ServerAddr:  "localhost",
			ServerPort:  2222,
			TUNName:     "ssh-vpn0",
			TUNAddr:     "10.8.0.2",
			TUNNetmask:  "255.255.255.0",
			MTU:         1400,
			AutoConnect: true,
		},
		Channels: ChannelsConfig{
			MinRead:     2,
			MaxRead:     8,
			MinWrite:    1,
			MaxWrite:    4,
			ReadRatio:   0.8,
			WriteRatio:  0.2,
			HealthCheck: 5 * time.Second,
			Timeout:     30 * time.Second,
		},
		Security: SecurityConfig{
			Encryption:  "aes256-gcm",
			Compression: "lz4",
		},
		Monitor: MonitorConfig{
			Enabled:    true,
			ListenAddr: "127.0.0.1",
			ListenPort: 9090,
		},
	}
}
