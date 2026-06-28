package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

type Tray struct {
	app *App
}

func NewTray(app *App) *Tray {
	return &Tray{app: app}
}

func (t *Tray) Setup() {
	time.Sleep(500 * time.Millisecond)

	desk, ok := t.app.fyneApp.(desktop.App)
	if !ok {
		t.app.logger.Warn("system tray not available - type assertion to desktop.App failed")
		return
	}

	t.app.logger.Info("system tray available, setting up menu")

	menu := fyne.NewMenu("SSH VPN",
		fyne.NewMenuItem("Show Window", func() {
			t.app.mainWin.Show()
			t.app.mainWin.RequestFocus()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Connect", func() {
			go t.app.StartClient()
		}),
		fyne.NewMenuItem("Disconnect", func() {
			go t.app.StopClient()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			t.app.StopClient()
			t.app.fyneApp.Quit()
		}),
	)

	desk.SetSystemTrayMenu(menu)
	t.app.logger.Info("system tray menu set successfully")
}

func (t *Tray) onExit() {
}
