//go:build windows

package tun

import (
	"go.uber.org/zap"
)

type RouteManager struct {
	savedRoutes []SavedRoute
	ifaceName   string
	subnet      string
	serverAddr  string
	logger      *zap.Logger
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
	return nil
}

func (rm *RouteManager) Restore() {
}
