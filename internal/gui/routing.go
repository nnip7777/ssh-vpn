package gui

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type AppInfo struct {
	Name string
	PID  int
	Icon string
}

type RoutingMode int

const (
	RoutingModeFull RoutingMode = iota
	RoutingModePerApp
	RoutingModeSplit
)

type RoutingState struct {
	Mode         RoutingMode
	SelectedApps map[string]bool
	ExcludedSubnets []string
	mu           sync.RWMutex
}

func (a *App) createRoutingTab() fyne.CanvasObject {
	titleLabel := widget.NewLabel("VPN Routing Mode")

	modeLabel := widget.NewLabel("Current mode: Full Tunnel")

	fullRadio := widget.NewRadioGroup([]string{"Full Tunnel (all traffic)", "Per-App (selected apps only)"}, func(value string) {
		if strings.HasPrefix(value, "Full") {
			a.routingState.mu.Lock()
			a.routingState.Mode = RoutingModeFull
			a.routingState.mu.Unlock()
			modeLabel.SetText("Current mode: Full Tunnel")
		} else {
			a.routingState.mu.Lock()
			a.routingState.Mode = RoutingModePerApp
			a.routingState.mu.Unlock()
			modeLabel.SetText("Current mode: Per-App")
		}
	})
	fullRadio.SetSelected("Full Tunnel (all traffic)")

	appListContainer := container.NewVBox()
	appScroll := container.NewVScroll(appListContainer)

	refreshBtn := widget.NewButton("Refresh Apps", func() {
		a.refreshAppList(appListContainer)
	})

	go a.autoRefreshApps(appListContainer)

	statusLabel := widget.NewLabel("")

	applyBtn := widget.NewButton("Apply Routing", func() {
		a.routingState.mu.RLock()
		mode := a.routingState.Mode
		a.routingState.mu.RUnlock()

		if mode == RoutingModePerApp {
			count := a.getSelectedAppCount()
			statusLabel.SetText(fmt.Sprintf("Per-App mode: %d apps selected for VPN routing", count))
		} else {
			statusLabel.SetText("Full Tunnel mode: all traffic routed through VPN")
		}
	})

	return container.NewVBox(
		titleLabel,
		modeLabel,
		fullRadio,
		widget.NewSeparator(),
		widget.NewLabel("Running Applications:"),
		container.NewGridWithColumns(2, refreshBtn, layout.NewSpacer()),
		appScroll,
		widget.NewSeparator(),
		applyBtn,
		statusLabel,
		layout.NewSpacer(),
	)
}

func (a *App) refreshAppList(list *fyne.Container) {
	apps := getRunningApps()

	list.Objects = nil

	if len(apps) == 0 {
		list.Objects = append(list.Objects, widget.NewLabel("No applications found"))
		list.Refresh()
		return
	}

	for _, app := range apps {
		appName := app.Name
		check := widget.NewCheck(appName, func(checked bool) {
			a.routingState.mu.Lock()
			a.routingState.SelectedApps[appName] = checked
			a.routingState.mu.Unlock()
		})

		a.routingState.mu.RLock()
		if a.routingState.SelectedApps[appName] {
			check.SetChecked(true)
		}
		a.routingState.mu.RUnlock()

		pidLabel := widget.NewLabel(fmt.Sprintf("PID: %d", app.PID))
		row := container.NewHBox(check, layout.NewSpacer(), pidLabel)
		list.Objects = append(list.Objects, row)
	}

	list.Refresh()
}

func (a *App) autoRefreshApps(list *fyne.Container) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if a.routingState != nil {
			a.routingState.mu.RLock()
			mode := a.routingState.Mode
			a.routingState.mu.RUnlock()
			if mode == RoutingModePerApp {
				a.refreshAppList(list)
			}
		}
	}
}

func (a *App) getSelectedAppCount() int {
	a.routingState.mu.RLock()
	defer a.routingState.mu.RUnlock()

	count := 0
	for _, selected := range a.routingState.SelectedApps {
		if selected {
			count++
		}
	}
	return count
}

func getRunningApps() []AppInfo {
	out, err := exec.Command("ps", "aux").CombinedOutput()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var apps []AppInfo

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		pid := 0
		fmt.Sscanf(fields[1], "%d", &pid)

		cmd := strings.Join(fields[10:], " ")

		name := extractAppName(cmd)
		if name == "" || name == "ps" || name == "grep" || name == "bash" || name == "zsh" ||
			name == "ssh" || name == "kernel_task" || name == "launchd" {
			continue
		}

		if seen[name] {
			continue
		}
		seen[name] = true

		apps = append(apps, AppInfo{
			Name: name,
			PID:  pid,
		})
	}

	return apps
}

func extractAppName(cmd string) string {
	parts := strings.Split(cmd, "/")
	base := parts[len(parts)-1]

	base = strings.TrimPrefix(base, "-")

	if idx := strings.Index(base, " "); idx != -1 {
		base = base[:idx]
	}

	return base
}

func (a *App) getSelectedAppRoutes() []string {
	a.routingState.mu.RLock()
	defer a.routingState.mu.RUnlock()

	var routes []string
	for appName, selected := range a.routingState.SelectedApps {
		if selected {
			connections := getAppConnections(appName)
			routes = append(routes, connections...)
		}
	}
	return routes
}

func getAppConnections(appName string) []string {
	out, err := exec.Command("lsof", "-i", "-P", "-n", "-c", appName).CombinedOutput()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var ips []string

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		addr := fields[8]
		parts := strings.Split(addr, ":")
		if len(parts) < 1 {
			continue
		}

		ip := parts[0]
		if ip == "*" || ip == "127.0.0.1" || ip == "::1" || ip == "0.0.0.0" {
			continue
		}

		if strings.Contains(ip, "->") {
			parts2 := strings.Split(ip, "->")
			ip = parts2[0]
		}

		if !seen[ip] {
			seen[ip] = true
			ips = append(ips, ip)
		}
	}

	return ips
}
