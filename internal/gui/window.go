package gui

import (
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (a *App) setupUI() {
	mainTab := container.NewTabItem("Main", a.createMainTab())
	monitorTab := container.NewTabItem("Monitor", a.createMonitorTab())
	routingTab := container.NewTabItem("Routing", a.createRoutingTab())
	settingsTab := container.NewTabItem("Settings", a.createSettingsTab())
	debugTab := container.NewTabItem("Debug", a.createDebugTab())
	helpTab := container.NewTabItem("Help", a.createHelpTab())

	quitBtn := widget.NewButton("Quit", func() {
		dialog.ShowConfirm("Quit", "Stop VPN and quit?", func(ok bool) {
			if ok {
				a.StopClient()
				a.fyneApp.Quit()
			}
		}, a.mainWin)
	})
	quitBtn.Importance = widget.DangerImportance

	tabs := container.NewAppTabs(mainTab, monitorTab, routingTab, settingsTab, debugTab, helpTab)
	tabs.SetTabLocation(container.TabLocationLeading)
	tabs.Append(container.NewTabItem("Quit", nil))

	tabs.OnChanged = func(tab *container.TabItem) {
		if tab.Text == "Quit" {
			dialog.ShowConfirm("Quit", "Stop VPN and quit?", func(ok bool) {
				if ok {
					a.StopClient()
					a.fyneApp.Quit()
				}
			}, a.mainWin)
			tabs.SelectIndex(0)
		}
	}

	a.mainWin.SetContent(tabs)

	a.mainWin.SetCloseIntercept(func() {
		dialog.ShowConfirm("Quit", "Stop VPN and quit?", func(ok bool) {
			if ok {
				a.StopClient()
				a.fyneApp.Quit()
			}
		}, a.mainWin)
	})
}

func (a *App) createMainTab() fyne.CanvasObject {
	statusLabel := widget.NewLabel("Disconnected")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	statusLabel.Importance = widget.DangerImportance

	serverLabel := widget.NewLabel(fmt.Sprintf("Server: %s:%d", a.config.Client.ServerAddr, a.config.Client.ServerPort))
	tunLabel := widget.NewLabel(fmt.Sprintf("TUN: %s (%s)", a.config.Client.TUNName, a.config.Client.TUNAddr))

	connectBtn := widget.NewButton("Connect", func() {
		go a.StartClient()
	})
	connectBtn.Importance = widget.HighImportance

	disconnectBtn := widget.NewButton("Disconnect", func() {
		go a.StopClient()
	})
	disconnectBtn.Importance = widget.DangerImportance

	go a.updateStatus(statusLabel)

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabel("SSH VPN Client"),
			widget.NewSeparator(),
		),
		container.NewHBox(connectBtn, disconnectBtn, layout.NewSpacer()),
		nil, nil,
		container.NewVBox(
			widget.NewCard("Status", "", statusLabel),
			widget.NewCard("Server", "", container.NewVBox(serverLabel, tunLabel)),
		),
	)
}

func (a *App) updateStatus(label *widget.Label) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if a.client != nil && a.client.IsConnected() {
			label.SetText("Connected")
			label.Importance = widget.SuccessImportance
		} else {
			label.SetText("Disconnected")
			label.Importance = widget.DangerImportance
		}
	}
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
