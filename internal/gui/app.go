package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"github.com/nnip7777/ssh-vpn/internal/client"
	"github.com/nnip7777/ssh-vpn/internal/config"
	"github.com/nnip7777/ssh-vpn/internal/version"
	"go.uber.org/zap"
)

type App struct {
	fyneApp      fyne.App
	mainWin      fyne.Window
	tray         *Tray
	updater      *Updater
	client       *client.Client
	config       *config.Config
	logger       *zap.Logger
	statusCh     chan StatusUpdate
	routingState *RoutingState
	monUI        *monitorUI
	contentTabs  *fyne.Container
	mainStatus   *canvas.Text
	monTotalIn   *canvas.Text
	monTotalOut  *canvas.Text
	monChannels  *canvas.Text
	monStopCh    chan struct{}
}

type StatusUpdate struct {
	Connected bool
	Running   bool
	Error     string
}

func New(cfg *config.Config, logger *zap.Logger) *App {
	fyneApp := app.NewWithID("com.sshvpn.client")
	fyneApp.Settings().SetTheme(&NeonTheme{})
	mainWin := fyneApp.NewWindow("SSH VPN Client")
	mainWin.Resize(fyne.NewSize(900, 600))

	a := &App{
		fyneApp:  fyneApp,
		mainWin:  mainWin,
		config:   cfg,
		logger:   logger,
		statusCh: make(chan StatusUpdate, 10),
		routingState: &RoutingState{
			Mode:         RoutingModeFull,
			SelectedApps: make(map[string]bool),
		},
	}

	a.tray = NewTray(a)
	a.updater = NewUpdater(version.Version, logger)

	return a
}

func (a *App) Run() {
	a.setupUI()
	a.mainWin.Show()
	go a.tray.Setup()
	a.fyneApp.Run()
}

func (a *App) GetVersion() string {
	return version.String()
}

func (a *App) GetStatus() StatusUpdate {
	select {
	case s := <-a.statusCh:
		return s
	default:
		return StatusUpdate{}
	}
}

func (a *App) StartClient() {
	if a.client != nil && a.client.IsConnected() {
		return
	}

	c, err := client.New(a.config, a.logger)
	if err != nil {
		a.logger.Error("failed to create client", zap.Error(err))
		a.statusCh <- StatusUpdate{Error: err.Error()}
		return
	}

	if err := c.Connect(); err != nil {
		a.logger.Error("failed to connect", zap.Error(err))
		a.statusCh <- StatusUpdate{Error: err.Error()}
		return
	}

	if err := c.Start(); err != nil {
		a.logger.Error("failed to start", zap.Error(err))
		a.statusCh <- StatusUpdate{Error: err.Error()}
		return
	}

	a.client = c
	a.monStopCh = make(chan struct{})
	a.statusCh <- StatusUpdate{Connected: true, Running: true}
	a.logger.Info("client started")
}

func (a *App) StopClient() {
	if a.client == nil {
		return
	}

	if a.monStopCh != nil {
		close(a.monStopCh)
		a.monStopCh = nil
	}

	a.client.Stop()
	a.client = nil
	a.statusCh <- StatusUpdate{Connected: false, Running: false}
	a.logger.Info("client stopped")
}
