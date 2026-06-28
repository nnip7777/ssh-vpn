package gui

import (
	"fmt"
	"image/color"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (a *App) setupUI() {
	header := a.createHeader()
	statusBar := a.createStatusBar()

	dashboardPages := []fyne.CanvasObject{
		a.createMainDashboard(),
		a.createMonitorDashboard(),
		a.createRoutingDashboard(),
		a.createSettingsDashboard(),
		a.createDebugDashboard(),
		a.createHelpDashboard(),
	}

	a.contentTabs = container.NewStack(dashboardPages...)

	sidebar := a.createSidebar(dashboardPages)

	sidebarBg := canvas.NewRectangle(color.NRGBA{R: 13, G: 17, B: 30, A: 255})
	sidebarArea := container.NewStack(sidebarBg, sidebar)

	contentBg := canvas.NewRectangle(darkBg)
	contentArea := container.NewStack(contentBg, a.contentTabs)

	mainArea := container.NewHSplit(sidebarArea, contentArea)
	mainArea.Offset = 140.0 / 900.0

	root := container.NewBorder(header, statusBar, nil, nil, mainArea)
	a.mainWin.SetContent(root)

	a.switchTab(0, dashboardPages)
	go a.statusUpdater()

	a.mainWin.SetCloseIntercept(func() {
		dialog.ShowConfirm("Quit", "Stop VPN and quit?", func(ok bool) {
			if ok {
				a.StopClient()
				a.fyneApp.Quit()
			}
		}, a.mainWin)
	})
}

func (a *App) statusUpdater() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.mainStatus == nil {
			continue
		}
		if a.client != nil && a.client.IsConnected() {
			a.mainStatus.Text = "CONNECTED"
			a.mainStatus.Color = neonGreen
		} else {
			a.mainStatus.Text = "DISCONNECTED"
			a.mainStatus.Color = dangerRed
		}
		a.mainStatus.Refresh()
	}
}

func (a *App) createHeader() fyne.CanvasObject {
	logo := canvas.NewText("SSH VPN", neonCyan)
	logo.TextSize = 20
	logo.TextStyle = fyne.TextStyle{Bold: true}

	versionText := canvas.NewText("v0.4.0", textGrey)
	versionText.TextSize = 12

	left := container.NewHBox(logo, versionText, layout.NewSpacer())

	statusText := canvas.NewText("SYSTEM_STATUS: OPERATIONAL", neonGreen)
	statusText.TextSize = 11
	right := container.NewHBox(statusText)

	return container.NewStack(
		canvas.NewRectangle(color.NRGBA{R: 13, G: 17, B: 30, A: 255}),
		container.NewBorder(nil, nil, left, right, nil),
	)
}

func (a *App) createSidebar(pages []fyne.CanvasObject) fyne.CanvasObject {
	labels := []string{"Main", "Monitor", "Routing", "Settings", "Debug", "Help"}
	var buttons []*widget.Button

	for i, label := range labels {
		idx := i
		btn := widget.NewButton(label, nil)
		btn.Importance = widget.LowImportance
		if idx == 0 {
			btn.Importance = widget.HighImportance
		}
		btn.OnTapped = func() {
			for j, b := range buttons {
				if j == idx {
					b.Importance = widget.HighImportance
				} else {
					b.Importance = widget.LowImportance
				}
				b.Refresh()
			}
			a.switchTab(idx, pages)
		}
		buttons = append(buttons, btn)
	}

	quitBtn := widget.NewButton("Quit", func() {
		dialog.ShowConfirm("Quit", "Stop VPN and quit?", func(ok bool) {
			if ok {
				a.StopClient()
				a.fyneApp.Quit()
			}
		}, a.mainWin)
	})
	quitBtn.Importance = widget.DangerImportance

	sidebarContent := container.NewVBox(layout.NewSpacer())
	for _, b := range buttons {
		sidebarContent.Objects = append(sidebarContent.Objects, b)
	}
	sidebarContent.Objects = append(sidebarContent.Objects,
		layout.NewSpacer(),
		quitBtn,
	)

	return container.NewPadded(sidebarContent)
}

func (a *App) switchTab(index int, pages []fyne.CanvasObject) {
	for i, p := range pages {
		if i == index {
			p.Show()
		} else {
			p.Hide()
		}
	}
}

func (a *App) createStatusBar() fyne.CanvasObject {
	timeText := canvas.NewText(fmt.Sprintf("UPTIME: %s", time.Now().Format("15:04:05")), textGrey)
	timeText.TextSize = 11

	statusText := canvas.NewText("SECURE_CONNECTION", neonGreen)
	statusText.TextSize = 11

	latencyText := canvas.NewText("LATENCY: --ms", textGrey)
	latencyText.TextSize = 11

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			timeText.Text = fmt.Sprintf("UPTIME: %s", time.Now().Format("15:04:05"))
			timeText.Refresh()
		}
	}()

	left := container.NewHBox(statusText, layout.NewSpacer(), latencyText)
	right := container.NewHBox(timeText)

	return container.NewStack(
		canvas.NewRectangle(color.NRGBA{R: 13, G: 17, B: 30, A: 255}),
		container.NewBorder(nil, nil, left, right, nil),
	)
}

func (a *App) createMainDashboard() fyne.CanvasObject {
	a.mainStatus = canvas.NewText("DISCONNECTED", dangerRed)
	a.mainStatus.TextSize = 22
	a.mainStatus.TextStyle = fyne.TextStyle{Bold: true}

	statusTitle := canvas.NewText("CONNECTION STATUS", textGrey)
	statusTitle.TextSize = 11

	statusBox := container.NewVBox(statusTitle, a.mainStatus)
	statusBg := canvas.NewRectangle(darkPanel)
	statusCard := container.NewStack(statusBg, container.NewPadded(statusBox))

	serverTitle := canvas.NewText("SERVER", textGrey)
	serverTitle.TextSize = 11
	serverVal := canvas.NewText(fmt.Sprintf("%s:%d", a.config.Client.ServerAddr, a.config.Client.ServerPort), neonCyan)
	serverVal.TextSize = 18
	serverVal.TextStyle = fyne.TextStyle{Bold: true}
	serverCard := container.NewStack(canvas.NewRectangle(darkPanel), container.NewPadded(container.NewVBox(serverTitle, serverVal)))

	tunTitle := canvas.NewText("TUN INTERFACE", textGrey)
	tunTitle.TextSize = 11
	tunVal := canvas.NewText(fmt.Sprintf("%s (%s)", a.config.Client.TUNName, a.config.Client.TUNAddr), neonMagenta)
	tunVal.TextSize = 18
	tunVal.TextStyle = fyne.TextStyle{Bold: true}
	tunCard := container.NewStack(canvas.NewRectangle(darkPanel), container.NewPadded(container.NewVBox(tunTitle, tunVal)))

	topRow := container.NewGridWithColumns(3, statusCard, serverCard, tunCard)

	connectBtn := widget.NewButton("CONNECT", func() {
		go a.StartClient()
	})
	connectBtn.Importance = widget.HighImportance

	disconnectBtn := widget.NewButton("DISCONNECT", func() {
		go a.StopClient()
	})
	disconnectBtn.Importance = widget.DangerImportance

	controlLabel := canvas.NewText("CONTROL INTERFACE", textGrey)
	controlLabel.TextSize = 11

	buttons := container.NewGridWithColumns(2, connectBtn, disconnectBtn)
	controlPanel := container.NewVBox(controlLabel, widget.NewSeparator(), buttons)
	bg := canvas.NewRectangle(darkPanel)
	panel := container.NewStack(bg, container.NewPadded(controlPanel))

	return container.NewBorder(topRow, nil, nil, nil, panel)
}

func (a *App) createMonitorDashboard() fyne.CanvasObject {
	title := canvas.NewText("NETWORK TRAFFIC ANALYTICS", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	a.monTotalIn = canvas.NewText("0 B", neonCyan)
	a.monTotalIn.TextSize = 14
	a.monTotalIn.TextStyle = fyne.TextStyle{Bold: true}

	a.monTotalOut = canvas.NewText("0 B", neonMagenta)
	a.monTotalOut.TextSize = 14
	a.monTotalOut.TextStyle = fyne.TextStyle{Bold: true}

	a.monChannels = canvas.NewText("R0 / W0", neonGreen)
	a.monChannels.TextSize = 14
	a.monChannels.TextStyle = fyne.TextStyle{Bold: true}

	topBar := container.NewHBox(
		container.NewHBox(canvas.NewText("IN:", textGrey), a.monTotalIn),
		canvas.NewText("  ", nil),
		container.NewHBox(canvas.NewText("OUT:", textGrey), a.monTotalOut),
		canvas.NewText("  ", nil),
		container.NewHBox(canvas.NewText("CH:", textGrey), a.monChannels),
	)

	channelPanel := NewNeonPanel("CHANNEL STATUS", a.createMonitorTab())

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator(), topBar),
		nil, nil, nil,
		channelPanel,
	)
}

func (a *App) createRoutingDashboard() fyne.CanvasObject {
	title := canvas.NewText("ROUTING CONFIGURATION", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		nil, nil, nil,
		a.createRoutingTab(),
	)
}

func (a *App) createSettingsDashboard() fyne.CanvasObject {
	title := canvas.NewText("SYSTEM SETTINGS", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		nil, nil, nil,
		a.createSettingsTab(),
	)
}

func (a *App) createDebugDashboard() fyne.CanvasObject {
	title := canvas.NewText("DIAGNOSTIC TOOLS", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		nil, nil, nil,
		a.createDebugTab(),
	)
}

func (a *App) createHelpDashboard() fyne.CanvasObject {
	title := canvas.NewText("ABOUT", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		nil, nil, nil,
		a.createHelpTab(),
	)
}

func (a *App) createSettingsTab() fyne.CanvasObject {
	serverEntry := widget.NewEntry()
	serverEntry.SetText(a.config.Client.ServerAddr)

	portEntry := widget.NewEntry()
	portEntry.SetText(fmt.Sprintf("%d", a.config.Client.ServerPort))

	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(a.config.Client.Username)

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetText(a.config.Client.Password)

	tunNameEntry := widget.NewEntry()
	tunNameEntry.SetText(a.config.Client.TUNName)

	tunAddrEntry := widget.NewEntry()
	tunAddrEntry.SetText(a.config.Client.TUNAddr)

	autoConnectCheck := widget.NewCheck("Auto-connect on startup", nil)
	autoConnectCheck.SetChecked(a.config.Client.AutoConnect)

	saveBtn := widget.NewButton("Save", func() {
		a.config.Client.ServerAddr = serverEntry.Text
		a.config.Client.ServerPort = parsePort(portEntry.Text)
		a.config.Client.Username = usernameEntry.Text
		a.config.Client.Password = passwordEntry.Text
		a.config.Client.TUNName = tunNameEntry.Text
		a.config.Client.TUNAddr = tunAddrEntry.Text
		a.config.Client.AutoConnect = autoConnectCheck.Checked
		dialog.ShowInformation("Settings", "Settings saved", a.mainWin)
	})
	saveBtn.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Server", serverEntry),
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("Username", usernameEntry),
		widget.NewFormItem("Password", passwordEntry),
		widget.NewFormItem("TUN Name", tunNameEntry),
		widget.NewFormItem("TUN Address", tunAddrEntry),
	)

	return container.NewBorder(nil, autoConnectCheck, nil, saveBtn, form)
}

func (a *App) createHelpTab() fyne.CanvasObject {
	versionLabel := widget.NewLabel(a.GetVersion())
	updateBtn := widget.NewButton("Check for Updates", func() {
		a.updater.CheckForUpdates(a.mainWin)
	})
	docURL, _ := url.Parse("https://github.com/nnip7777/ssh-vpn")

	return container.NewVBox(
		widget.NewLabel("SSH VPN Client"),
		widget.NewSeparator(),
		widget.NewLabel("Version:"),
		versionLabel,
		updateBtn,
		widget.NewHyperlink("Documentation", docURL),
		layout.NewSpacer(),
	)
}

func parsePort(s string) int {
	var port int
	fmt.Sscanf(s, "%d", &port)
	if port == 0 {
		port = 2222
	}
	return port
}
