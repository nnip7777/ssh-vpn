package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nnip7777/ssh-vpn/internal/config"
	"github.com/nnip7777/ssh-vpn/internal/version"
	"gopkg.in/yaml.v3"
)

const banner = `
╔══════════════════════════════════════════════════╗
║       SSH VPN Client Configurator  v%s       ║
╚══════════════════════════════════════════════════╝
`

var reader = bufio.NewReader(os.Stdin)

func main() {
	outputPath := "client.yaml"

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-output=") {
			outputPath = strings.TrimPrefix(arg, "-output=")
		}
	}

	fmt.Printf(banner, version.Version)

	cfg := config.DefaultConfig()
	cfg.Channels.HealthCheck = 5000000000
	cfg.Channels.Timeout = 30000000000

	runWizard(cfg)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
		os.Exit(1)
	}

	absPath, _ := filepath.Abs(outputPath)
	fmt.Printf("\n✓ Configuration saved to: %s\n", absPath)
	fmt.Println("\nStart with:")
	fmt.Printf("  sudo ./ssh-vpn-client -config %s\n", outputPath)
}

func runWizard(cfg *config.Config) {
	fmt.Println("\n── Step 1: Server Connection ──")

	cfg.Client.ServerAddr = promptString("Server address", cfg.Client.ServerAddr)
	cfg.Client.ServerPort = promptInt("Server port", cfg.Client.ServerPort)

	fmt.Println("\n── Step 2: Authentication ──")

	cfg.Client.Username = promptString("SSH username", cfg.Client.Username)

	fmt.Println("  Authentication method:")
	fmt.Println("    [1] SSH Key (recommended)")
	fmt.Println("    [2] Password")
	method := promptChoice("Choice", 1, 2)

	if method == 2 {
		cfg.Client.Password = promptString("SSH password", "")
	} else {
		cfg.Client.PrivateKeyPath = promptString("Private key path", cfg.Client.PrivateKeyPath)
	}

	fmt.Println("\n── Step 3: Network ──")

	cfg.Client.TUNAddr = promptString("Your VPN IP", cfg.Client.TUNAddr)
	cfg.Client.TUNNetmask = promptString("Netmask", cfg.Client.TUNNetmask)
	cfg.Client.MTU = promptInt("MTU", cfg.Client.MTU)

	fmt.Println("\n── Step 4: Channel Settings ──")

	fmt.Println("  Channel mode:")
	fmt.Println("    [1] Home      - fast internet, 4-8 read channels")
	fmt.Println("    [2] Office    - corporate, 2-4 read channels")
	fmt.Println("    [3] Mobile    - cellular, 2-6 read channels")
	fmt.Println("    [4] Custom    - manual configuration")
	mode := promptChoice("Mode", 1, 4)

	switch mode {
	case 1:
		cfg.Channels.MinRead = 4
		cfg.Channels.MaxRead = 8
		cfg.Channels.MinWrite = 2
		cfg.Channels.MaxWrite = 4
		cfg.Channels.ReadRatio = 0.8
		cfg.Channels.WriteRatio = 0.2
	case 2:
		cfg.Channels.MinRead = 2
		cfg.Channels.MaxRead = 4
		cfg.Channels.MinWrite = 1
		cfg.Channels.MaxWrite = 2
		cfg.Channels.ReadRatio = 0.7
		cfg.Channels.WriteRatio = 0.3
	case 3:
		cfg.Channels.MinRead = 2
		cfg.Channels.MaxRead = 6
		cfg.Channels.MinWrite = 1
		cfg.Channels.MaxWrite = 3
		cfg.Channels.ReadRatio = 0.85
		cfg.Channels.WriteRatio = 0.15
	case 4:
		cfg.Channels.MinRead = promptInt("Min read channels", cfg.Channels.MinRead)
		cfg.Channels.MaxRead = promptInt("Max read channels", cfg.Channels.MaxRead)
		cfg.Channels.MinWrite = promptInt("Min write channels", cfg.Channels.MinWrite)
		cfg.Channels.MaxWrite = promptInt("Max write channels", cfg.Channels.MaxWrite)
		cfg.Channels.ReadRatio = promptFloat("Read ratio (0.0-1.0)", cfg.Channels.ReadRatio)
		cfg.Channels.WriteRatio = 1.0 - cfg.Channels.ReadRatio
		fmt.Printf("  Write ratio set to: %.2f\n", cfg.Channels.WriteRatio)
	}

	fmt.Println("\n── Step 5: Security ──")

	fmt.Println("  Compression:")
	fmt.Println("    [1] LZ4 (recommended, faster)")
	fmt.Println("    [2] None (less CPU usage)")
	comp := promptChoice("Choice", 1, 2)
	if comp == 1 {
		cfg.Security.Compression = "lz4"
	} else {
		cfg.Security.Compression = "none"
	}

	fmt.Println("\n── Step 6: Connection Behavior ──")

	cfg.Client.AutoConnect = promptYesNo("Auto-connect on start", true)

	fmt.Println("\n── Summary ──")

	printSummary(cfg)
}

func promptString(prompt, defaultVal string) string {
	fmt.Printf("  %s [%s]: ", prompt, defaultVal)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func promptInt(prompt string, defaultVal int) int {
	for {
		fmt.Printf("  %s [%d]: ", prompt, defaultVal)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal
		}
		val, err := strconv.Atoi(line)
		if err != nil {
			fmt.Printf("  ✗ Invalid number\n")
			continue
		}
		return val
	}
}

func promptFloat(prompt string, defaultVal float64) float64 {
	for {
		fmt.Printf("  %s [%.2f]: ", prompt, defaultVal)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal
		}
		val, err := strconv.ParseFloat(line, 64)
		if err != nil {
			fmt.Printf("  ✗ Invalid number\n")
			continue
		}
		if val < 0 || val > 1 {
			fmt.Printf("  ✗ Must be between 0.0 and 1.0\n")
			continue
		}
		return val
	}
}

func promptChoice(prompt string, min, max int) int {
	for {
		fmt.Printf("  %s [%d-%d]: ", prompt, min, max)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return min
		}
		val, err := strconv.Atoi(line)
		if err != nil || val < min || val > max {
			fmt.Printf("  ✗ Enter %d-%d\n", min, max)
			continue
		}
		return val
	}
}

func promptYesNo(prompt string, defaultVal bool) bool {
	def := "y/N"
	if defaultVal {
		def = "Y/n"
	}
	fmt.Printf("  %s [%s]: ", prompt, def)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultVal
	}
	return line == "y" || line == "yes"
}

func printSummary(cfg *config.Config) {
	fmt.Printf("  Server:     %s:%d\n", cfg.Client.ServerAddr, cfg.Client.ServerPort)
	fmt.Printf("  Username:   %s\n", cfg.Client.Username)
	if cfg.Client.Password != "" {
		fmt.Printf("  Auth:       Password\n")
	} else {
		fmt.Printf("  Auth:       Key (%s)\n", cfg.Client.PrivateKeyPath)
	}
	fmt.Printf("  VPN IP:     %s/%s\n", cfg.Client.TUNAddr, cfg.Client.TUNNetmask)
	fmt.Printf("  MTU:        %d\n", cfg.Client.MTU)
	fmt.Printf("  Channels:   %d-%d read, %d-%d write\n",
		cfg.Channels.MinRead, cfg.Channels.MaxRead,
		cfg.Channels.MinWrite, cfg.Channels.MaxWrite)
	fmt.Printf("  Ratio:      %.0f%% read, %.0f%% write\n",
		cfg.Channels.ReadRatio*100, cfg.Channels.WriteRatio*100)
	fmt.Printf("  Compression: %s\n", cfg.Security.Compression)
	fmt.Printf("  Auto-connect: %v\n", cfg.Client.AutoConnect)
}
