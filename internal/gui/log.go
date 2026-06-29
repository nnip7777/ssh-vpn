package gui

import (
	"compress/gzip"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/nnip7777/ssh-vpn/internal/version"
)

type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarn
	LogError
	LogDebug
	LogSuccess
)

type LogEntry struct {
	Time      time.Time
	Level     LogLevel
	Status    string
	Module    string
	Message   string
}

type LogManager struct {
	mu      sync.Mutex
	entries []LogEntry
	file    *os.File
	maxSize int
}

func (lm *LogManager) Init() error {
	logDir := "log"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	lm.gzipOldLogs(logDir)

	dateStr := time.Now().Format("2006-01-02")
	fileName := filepath.Join(logDir, fmt.Sprintf("ssh-vpn_%s.log", dateStr))

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	lm.file = f
	lm.maxSize = 10000

	if info, _ := os.Stat(fileName); info != nil && info.Size() > 0 {
		f.WriteString(fmt.Sprintf("\n--- session start %s ---\n", time.Now().Format("15:04:05")))
	}

	return nil
}

func (lm *LogManager) gzipOldLogs(logDir string) {
	today := time.Now().Format("2006-01-02")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "ssh-vpn_") || !strings.HasSuffix(name, ".log") {
			continue
		}
		datePart := strings.TrimPrefix(name, "ssh-vpn_")
		datePart = strings.TrimSuffix(datePart, ".log")
		if datePart == today {
			continue
		}
		srcPath := filepath.Join(logDir, name)
		dstPath := srcPath + ".gz"
		if _, err := os.Stat(dstPath); err == nil {
			continue
		}
		lm.gzipFile(srcPath, dstPath)
	}
}

func (lm *LogManager) gzipFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	if _, err := io.Copy(gw, in); err != nil {
		os.Remove(dst)
		return
	}
	gw.Close()
	out.Close()
	os.Remove(src)
}

func (lm *LogManager) Add(level LogLevel, status, module, message string) {
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Status:  status,
		Module:  module,
		Message: message,
	}
	lm.mu.Lock()
	lm.entries = append(lm.entries, entry)
	if len(lm.entries) > lm.maxSize {
		lm.entries = lm.entries[len(lm.entries)-lm.maxSize:]
	}
	lm.mu.Unlock()

	ts := entry.Time.Format("15:04:05.000")
	line := fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n", ts, levelStr(level), status, module, message)
	if lm.file != nil {
		lm.file.WriteString(line)
	}
}

func (lm *LogManager) GetAll() []LogEntry {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	out := make([]LogEntry, len(lm.entries))
	copy(out, lm.entries)
	return out
}

func (lm *LogManager) GetFiltered(level LogLevel, search string) []LogEntry {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	var out []LogEntry
	for _, e := range lm.entries {
		if level != -1 && e.Level != level {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(search)) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (lm *LogManager) Clear() {
	lm.mu.Lock()
	lm.entries = nil
	lm.mu.Unlock()
}

func (lm *LogManager) Close() {
	if lm.file != nil {
		lm.file.Close()
	}
}

func levelStr(l LogLevel) string {
	switch l {
	case LogInfo:
		return "INFO"
	case LogWarn:
		return "WARN"
	case LogError:
		return "ERROR"
	case LogDebug:
		return "DEBUG"
	case LogSuccess:
		return "OK"
	default:
		return "?"
	}
}

func levelColor(l LogLevel) color.Color {
	switch l {
	case LogInfo:
		return color.NRGBA{R: 120, G: 160, B: 180, A: 255}
	case LogWarn:
		return color.NRGBA{R: 180, G: 160, B: 80, A: 255}
	case LogError:
		return color.NRGBA{R: 180, G: 80, B: 80, A: 255}
	case LogDebug:
		return color.NRGBA{R: 100, G: 100, B: 120, A: 255}
	case LogSuccess:
		return color.NRGBA{R: 80, G: 160, B: 100, A: 255}
	default:
		return textGrey
	}
}

type logUI struct {
	manager       *LogManager
	list          *widget.List
	searchEntry   *widget.Entry
	levelSelect   *widget.Select
	fontSizeSlider *widget.Slider
	fontSizeLabel *canvas.Text
	fontSize      float64
	autoScroll    bool
	autoScrollChk *widget.Check
	selected      map[int]bool
	refreshCh     chan struct{}
}

func (a *App) createLogDashboard() fyne.CanvasObject {
	title := canvas.NewText("EVENT LOG", neonCyan)
	title.TextSize = 16
	title.TextStyle = fyne.TextStyle{Bold: true}

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		nil, nil, nil,
		a.createLogTab(),
	)
}

func (a *App) createLogTab() fyne.CanvasObject {
	lm := &LogManager{}
	if err := lm.Init(); err != nil {
		lm = &LogManager{}
	}
	a.logManager = lm

	ui := &logUI{
		manager:   lm,
		fontSize:  12,
		autoScroll: true,
		selected:  make(map[int]bool),
		refreshCh: make(chan struct{}, 1),
	}

	ui.searchEntry = widget.NewEntry()
	ui.searchEntry.SetPlaceHolder("Filter logs...")
	ui.searchEntry.OnChanged = func(s string) {
		ui.refreshList()
	}

	ui.levelSelect = widget.NewSelect([]string{"ALL", "INFO", "WARN", "ERROR", "DEBUG", "OK"}, func(s string) {
		ui.refreshList()
	})
	ui.levelSelect.SetSelected("ALL")

	ui.fontSizeSlider = widget.NewSlider(8, 20)
	ui.fontSizeSlider.Value = 12
	ui.fontSizeSlider.Step = 1
	ui.fontSizeLabel = canvas.NewText("12px", textGrey)
	ui.fontSizeLabel.TextSize = 11
	ui.fontSizeSlider.OnChanged = func(v float64) {
		ui.fontSize = v
		ui.fontSizeLabel.Text = fmt.Sprintf("%.0fpx", v)
		ui.fontSizeLabel.Refresh()
		ui.refreshList()
	}

	ui.autoScrollChk = widget.NewCheck("Auto-scroll", func(b bool) {
		ui.autoScroll = b
	})
	ui.autoScrollChk.SetChecked(true)

	ui.list = widget.NewList(
		func() int {
			return len(ui.getFiltered())
		},
		func() fyne.CanvasObject {
			return createLogRow(ui)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			updateLogRow(obj, ui, id)
		},
	)
	ui.list.OnSelected = func(id widget.ListItemID) {
		ui.selected[id] = true
	}
	ui.list.OnUnselected = func(id widget.ListItemID) {
		delete(ui.selected, id)
	}

	filterBar := container.NewHBox(
		ui.searchEntry,
		ui.levelSelect,
		layout.NewSpacer(),
		canvas.NewText("Font:", textGrey),
		ui.fontSizeSlider,
		ui.fontSizeLabel,
		ui.autoScrollChk,
	)

	clearBtn := widget.NewButton("Clear", func() {
		lm.Clear()
		ui.selected = make(map[int]bool)
		ui.refreshList()
	})

	copyAllBtn := widget.NewButton("Copy All", func() {
		entries := ui.getFiltered()
		var sb strings.Builder
		for _, e := range entries {
			sb.WriteString(formatLogLine(e))
		}
		a.mainWin.Clipboard().SetContent(sb.String())
	})

	copySelBtn := widget.NewButton("Copy Selected", func() {
		entries := ui.getFiltered()
		var sb strings.Builder
		for id := range ui.selected {
			if id < len(entries) {
				sb.WriteString(formatLogLine(entries[id]))
			}
		}
		if sb.Len() > 0 {
			a.mainWin.Clipboard().SetContent(sb.String())
		}
	})

	saveAllBtn := widget.NewButton("Save All", func() {
		entries := ui.getFiltered()
		a.saveLogToFile(entries, "all")
	})

	saveSelBtn := widget.NewButton("Save Selected", func() {
		entries := ui.getFiltered()
		var sel []LogEntry
		for id := range ui.selected {
			if id < len(entries) {
				sel = append(sel, entries[id])
			}
		}
		if len(sel) > 0 {
			a.saveLogToFile(sel, "selected")
		}
	})

	buttons := container.NewHBox(
		clearBtn, copyAllBtn, copySelBtn, layout.NewSpacer(), saveAllBtn, saveSelBtn,
	)

	go ui.autoRefreshLoop(a)

	return container.NewBorder(
		container.NewVBox(filterBar, widget.NewSeparator(), buttons),
		nil, nil, nil,
		ui.list,
	)
}

func createLogRow(ui *logUI) fyne.CanvasObject {
	timeLabel := canvas.NewText("", textGrey)
	statusLabel := canvas.NewText("", textGrey)
	moduleLabel := canvas.NewText("", textGrey)
	msgLabel := canvas.NewText("", textWhite)

	timeLabel.TextStyle = fyne.TextStyle{Monospace: true}
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	moduleLabel.TextStyle = fyne.TextStyle{Monospace: true}

	timeLabel.TextSize = 11
	statusLabel.TextSize = 11
	moduleLabel.TextSize = 11
	msgLabel.TextSize = 11

	rowBg := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	rowBg.Resize(fyne.NewSize(800, 28))

	hbox := container.NewHBox(
		timeLabel,
		canvas.NewText(" ", nil),
		statusLabel,
		canvas.NewText(" ", nil),
		moduleLabel,
		canvas.NewText(" ", nil),
		msgLabel,
	)

	return container.NewStack(rowBg, hbox)
}

func updateLogRow(obj fyne.CanvasObject, ui *logUI, id widget.ListItemID) {
	entries := ui.getFiltered()
	if id >= len(entries) {
		return
	}
	e := entries[id]

	stack := obj.(*fyne.Container)
	bg := stack.Objects[0].(*canvas.Rectangle)
	hbox := stack.Objects[1].(*fyne.Container)

	if len(hbox.Objects) < 7 {
		return
	}

	timeLabel := hbox.Objects[0].(*canvas.Text)
	statusLabel := hbox.Objects[2].(*canvas.Text)
	moduleLabel := hbox.Objects[4].(*canvas.Text)
	msgLabel := hbox.Objects[6].(*canvas.Text)

	fontSize := float32(int(ui.fontSize))

	timeLabel.Text = e.Time.Format("15:04:05.000")
	timeLabel.TextSize = fontSize - 1

	statusLabel.Text = fmt.Sprintf("%-5s", levelStr(e.Level))
	statusLabel.Color = levelColor(e.Level)
	statusLabel.TextSize = fontSize - 1

	moduleLabel.Text = e.Module
	moduleLabel.Color = cyanDim
	moduleLabel.TextSize = fontSize - 1

	msgLabel.Text = e.Message
	msgLabel.Color = textWhite
	msgLabel.TextSize = fontSize

	if ui.selected[id] {
		bg.FillColor = color.NRGBA{R: 0, G: 80, B: 120, A: 80}
	} else if id%2 == 0 {
		bg.FillColor = color.NRGBA{R: 10, G: 14, B: 26, A: 255}
	} else {
		bg.FillColor = color.NRGBA{R: 14, G: 18, B: 30, A: 255}
	}
	bg.Refresh()
	timeLabel.Refresh()
	statusLabel.Refresh()
	moduleLabel.Refresh()
	msgLabel.Refresh()
}

func (ui *logUI) getFiltered() []LogEntry {
	level := -1
	switch ui.levelSelect.Selected {
	case "INFO":
		level = int(LogInfo)
	case "WARN":
		level = int(LogWarn)
	case "ERROR":
		level = int(LogError)
	case "DEBUG":
		level = int(LogDebug)
	case "OK":
		level = int(LogSuccess)
	}
	return ui.manager.GetFiltered(LogLevel(level), ui.searchEntry.Text)
}

func (ui *logUI) refreshList() {
	if ui.list == nil {
		return
	}
	ui.list.Refresh()
}

func (ui *logUI) autoRefreshLoop(a *App) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		ui.refreshList()
		if ui.autoScroll && ui.list != nil && len(ui.getFiltered()) > 0 {
			ui.list.ScrollToBottom()
		}
	}
}

func formatLogLine(e LogEntry) string {
	return fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
		e.Time.Format("2006-01-02 15:04:05.000"),
		levelStr(e.Level),
		e.Status,
		e.Module,
		e.Message,
	)
}

func (a *App) saveLogToFile(entries []LogEntry, suffix string) {
	dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil || uc == nil {
			return
		}
		var sb strings.Builder
		for _, e := range entries {
			sb.WriteString(formatLogLine(e))
		}
		uc.Write([]byte(sb.String()))
		uc.Close()
	}, a.mainWin)
}

func (a *App) LogInfo(status, module, msg string) {
	if a.logManager != nil {
		a.logManager.Add(LogInfo, status, module, msg)
	}
}

func (a *App) LogWarn(status, module, msg string) {
	if a.logManager != nil {
		a.logManager.Add(LogWarn, status, module, msg)
	}
}

func (a *App) LogError(status, module, msg string) {
	if a.logManager != nil {
		a.logManager.Add(LogError, status, module, msg)
	}
}

func (a *App) LogDebug(status, module, msg string) {
	if a.logManager != nil {
		a.logManager.Add(LogDebug, status, module, msg)
	}
}

func (a *App) LogSuccess(status, module, msg string) {
	if a.logManager != nil {
		a.logManager.Add(LogSuccess, status, module, msg)
	}
}

func (a *App) initLogging() {
	a.LogInfo("START", version.Short(), "Application initialized")
	a.LogSuccess("START", version.Short(), fmt.Sprintf("Version %s loaded", version.Version))
	a.LogInfo("START", "GUI", "Dashboard ready")
}
