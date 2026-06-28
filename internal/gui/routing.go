package gui

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type AppInfo struct {
	Name string
	PID  int
	Type string
}

type RoutingMode int

const (
	RoutingModeFull RoutingMode = iota
	RoutingModePerApp
)

type RoutingState struct {
	Mode            RoutingMode
	SelectedApps    map[string]bool
	ExcludedSubnets []string
	mu              sync.RWMutex
}

var systemServices = map[string]bool{
	"kernel_task": true, "launchd": true, "WindowServer": true,
	"systemstats": true, "distnoted": true, "cfprefsd": true,
	"CoreServicesD": true, "launchservicesd": true, "backupd": true,
	"mds": true, "mds_stores": true, "spotlight": true,
	"photoanalysisd": true, "photolibraryd": true,
	"nsurlsessiond": true, "nsurlstoraged": true,
	"softwareupdated": true, "installd": true,
	"securityd": true, "ccsd": true, "trustd": true,
	"cloudd": true, "fmfd": true, "familycircled": true,
	"accountsd": true, "identityservicesd": true,
	"commcenter": true, "TelephonyUtilities": true,
	"mDNSResponder": true, "dnsproxyd": true,
	"syslogd": true, "os_log": true,
	"ActivityMonitor": true, "System Information": true,
	"QuickLookUIService": true, "UserEventAgent": true,
	"coreaudiod": true, "audioredhookd": true,
	"diskarbitrationd": true, "diskmanagementd": true,
	"powerd": true, "thermalmonitord": true,
	"configd": true, "networkd": true,
	"iapd": true, "iaphelper": true,
}

func (a *App) createRoutingTab() fyne.CanvasObject {
	modeLabel := canvas.NewText("Current mode: Full Tunnel (all traffic)", textGrey)
	modeLabel.TextSize = 12

	fullRadio := widget.NewRadioGroup([]string{"Full Tunnel", "Per-App (selected apps only)"}, func(value string) {
		if strings.HasPrefix(value, "Full") {
			a.routingState.mu.Lock()
			a.routingState.Mode = RoutingModeFull
			a.routingState.mu.Unlock()
			modeLabel.Text = "Current mode: Full Tunnel (all traffic)"
			modeLabel.Color = textGrey
			modeLabel.Refresh()
		} else {
			a.routingState.mu.Lock()
			a.routingState.Mode = RoutingModePerApp
			a.routingState.mu.Unlock()
			modeLabel.Text = "Current mode: Per-App (checked = VPN)"
			modeLabel.Color = neonCyan
			modeLabel.Refresh()
		}
	})
	fullRadio.SetSelected("Full Tunnel")

	systemList := container.NewVBox()
	userList := container.NewVBox()
	systemScroll := container.NewVScroll(systemList)
	systemScroll.SetMinSize(fyne.NewSize(0, 120))
	userScroll := container.NewVScroll(userList)
	userScroll.SetMinSize(fyne.NewSize(0, 200))

	refreshBtn := widget.NewButton("Refresh", func() {
		a.refreshAppList(systemList, userList)
	})

	go a.autoRefreshApps(systemList, userList)

	statusLabel := canvas.NewText("", textGrey)
	statusLabel.TextSize = 11

	applyBtn := widget.NewButton("Apply Routing", func() {
		a.routingState.mu.RLock()
		mode := a.routingState.Mode
		a.routingState.mu.RUnlock()

		if mode == RoutingModePerApp {
			count := a.getSelectedAppCount()
			statusLabel.Text = fmt.Sprintf("%d apps → VPN", count)
			statusLabel.Color = neonGreen
		} else {
			statusLabel.Text = "All traffic → VPN"
			statusLabel.Color = neonGreen
		}
		statusLabel.Refresh()
	})
	applyBtn.Importance = widget.HighImportance

	hint := canvas.NewText("✓ = VPN tunnel   ·   unchecked = direct", textGrey)
	hint.TextSize = 10

	sysTitle := canvas.NewText("SYSTEM SERVICES", textGrey)
	sysTitle.TextSize = 11
	sysTitle.TextStyle = fyne.TextStyle{Bold: true}

	userTitle := canvas.NewText("APPLICATIONS", neonCyan)
	userTitle.TextSize = 11
	userTitle.TextStyle = fyne.TextStyle{Bold: true}

	topPanel := container.NewVBox(
		modeLabel,
		fullRadio,
		widget.NewSeparator(),
		hint,
		container.NewBorder(nil, nil, nil, refreshBtn, widget.NewLabel("")),
		widget.NewSeparator(),
	)

	sysSection := container.NewVBox(sysTitle, systemScroll)
	userSection := container.NewVBox(userTitle, userScroll)

	bottomBar := container.NewBorder(nil, nil, applyBtn, statusLabel)

	listArea := container.NewVBox(sysSection, widget.NewSeparator(), userSection)

	return container.NewBorder(
		topPanel,
		bottomBar,
		nil, nil,
		listArea,
	)
}

func (a *App) refreshAppList(systemList, userList *fyne.Container) {
	apps := getRunningApps()

	systemList.Objects = nil
	userList.Objects = nil

	if len(apps) == 0 {
		userList.Objects = append(userList.Objects, widget.NewLabel("No applications found"))
		userList.Refresh()
		systemList.Refresh()
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

		pidLabel := canvas.NewText(fmt.Sprintf("PID:%d", app.PID), textGrey)
		pidLabel.TextSize = 10

		row := container.NewBorder(nil, nil, check, pidLabel)

		if app.Type == "system" {
			systemList.Objects = append(systemList.Objects, row)
		} else {
			userList.Objects = append(userList.Objects, row)
		}
	}

	systemList.Refresh()
	userList.Refresh()
}

func (a *App) autoRefreshApps(systemList, userList *fyne.Container) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if a.routingState != nil {
			a.routingState.mu.RLock()
			mode := a.routingState.Mode
			a.routingState.mu.RUnlock()
			if mode == RoutingModePerApp {
				a.refreshAppList(systemList, userList)
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
			name == "ssh" {
			continue
		}

		if seen[name] {
			continue
		}
		seen[name] = true

		appType := "user"
		if systemServices[name] {
			appType = "system"
		}

		apps = append(apps, AppInfo{
			Name: name,
			PID:  pid,
			Type: appType,
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
