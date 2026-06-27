package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	"go.uber.org/zap"
)

type RouteManager struct {
	savedRoutes []SavedRoute
	ifaceName   string
	subnet      string
	serverAddr  string
	logger      *zap.Logger
	mu          sync.Mutex
}

type SavedRoute struct {
	Dest   string
	Gw     string
	Flags  string
	IfName string
}

func NewRouteManager(ifaceName, subnet, serverAddr string, logger *zap.Logger) *RouteManager {
	return &RouteManager{
		ifaceName:  ifaceName,
		subnet:     subnet,
		serverAddr: serverAddr,
		logger:     logger,
	}
}

func (rm *RouteManager) SaveAndSetup() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.logger.Info("saving current routes")
	if err := rm.saveRoutes(); err != nil {
		rm.logger.Warn("failed to save routes", zap.Error(err))
	}

	rm.logger.Info("adding VPN routes")
	if err := rm.addVPNRoutes(); err != nil {
		return fmt.Errorf("failed to add VPN routes: %w", err)
	}

	return nil
}

func (rm *RouteManager) Restore() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.logger.Info("removing VPN routes")
	rm.removeVPNRoutes()

	rm.logger.Info("restoring saved routes")
	rm.restoreRoutes()
}

func (rm *RouteManager) saveRoutes() error {
	out, err := exec.Command("netstat", "-nr").CombinedOutput()
	if err != nil {
		return err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Internet:") || strings.HasPrefix(line, "Destination") ||
			strings.HasPrefix(line, "default") || strings.Contains(line, "lo0") ||
			strings.Contains(line, "utun") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		route := SavedRoute{
			Dest:   fields[0],
			Gw:     fields[1],
			Flags:  fields[2],
			IfName: fields[len(fields)-1],
		}
		rm.savedRoutes = append(rm.savedRoutes, route)
	}

	rm.logger.Info("saved routes", zap.Int("count", len(rm.savedRoutes)))
	return nil
}

func (rm *RouteManager) getDefaultGateway() string {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		rm.logger.Warn("failed to get default gateway", zap.Error(err))
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gw := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			if gw != "" && gw != "link#0" {
				rm.logger.Info("detected default gateway", zap.String("gateway", gw))
				return gw
			}
		}
	}

	return ""
}

func (rm *RouteManager) addVPNRoutes() error {
	defaultGW := rm.getDefaultGateway()

	if defaultGW != "" && rm.serverAddr != "" {
		serverIP := net.ParseIP(rm.serverAddr)
		if serverIP != nil {
			rm.logger.Info("adding route for server via default gateway",
				zap.String("server", rm.serverAddr),
				zap.String("gateway", defaultGW))
			cmd := []string{"route", "-n", "add", "-host", rm.serverAddr, defaultGW}
			if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
				rm.logger.Warn("failed to add server route",
					zap.String("cmd", strings.Join(cmd, " ")),
					zap.String("output", string(out)), zap.Error(err))
			}
		}
	}

	commands := [][]string{
		{"route", "-n", "add", "-net", "0.0.0.0/1", "-interface", rm.ifaceName},
		{"route", "-n", "add", "-net", "128.0.0.0/1", "-interface", rm.ifaceName},
	}

	for _, cmd := range commands {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			rm.logger.Warn("route add failed", zap.String("cmd", strings.Join(cmd, " ")),
				zap.String("output", string(out)), zap.Error(err))
		}
	}

	return nil
}

func (rm *RouteManager) removeVPNRoutes() {
	commands := [][]string{
		{"route", "-n", "delete", "-net", "0.0.0.0/1"},
		{"route", "-n", "delete", "-net", "128.0.0.0/1"},
		{"route", "-n", "delete", "-net", rm.subnet},
	}

	if rm.serverAddr != "" {
		commands = append(commands, []string{"route", "-n", "delete", "-host", rm.serverAddr})
	}

	for _, cmd := range commands {
		exec.Command(cmd[0], cmd[1:]...).Run()
	}
}

func (rm *RouteManager) restoreRoutes() {
	for _, route := range rm.savedRoutes {
		cmd := []string{"route", "-n", "add", "-net", route.Dest}
		if route.Gw != "" && route.Gw != "link#0" && !strings.HasPrefix(route.Gw, "link#") {
			cmd = append(cmd, route.Gw)
		} else if route.IfName != "" {
			cmd = append(cmd, "-interface", route.IfName)
		}
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			rm.logger.Debug("route restore failed", zap.String("cmd", strings.Join(cmd, " ")),
				zap.String("output", string(out)))
		}
	}
}
