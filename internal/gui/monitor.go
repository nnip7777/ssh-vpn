package gui

import (
	"fmt"
	"image/color"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/nnip7777/ssh-vpn/internal/channel"
)

type monitorUI struct {
	statusLabel *widget.Label
	infoLabel   *canvas.Text
	totalBar    *widget.ProgressBar
	readBar     *widget.ProgressBar
	writeBar    *widget.ProgressBar
	readLabel   *canvas.Text
	writeLabel  *canvas.Text
	channelList *fyne.Container
	prevTotal   uint64
	prevTime    time.Time
}

type channelSnapshot struct {
	id       uint16
	chType   string
	bytesIn  uint64
	bytesOut uint64
	pktsIn   uint64
	pktsOut  uint64
}

func (a *App) createMonitorTab() fyne.CanvasObject {
	m := &monitorUI{
		statusLabel: widget.NewLabel("Disconnected"),
		infoLabel:   canvas.NewText("", textGrey),
		totalBar:    widget.NewProgressBar(),
		readBar:     widget.NewProgressBar(),
		writeBar:    widget.NewProgressBar(),
		readLabel:   canvas.NewText("READ 0%", neonCyan),
		writeLabel:  canvas.NewText("WRITE 0%", neonMagenta),
		channelList: container.NewVBox(),
		prevTime:    time.Now(),
	}

	a.monUI = m
	m.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	m.infoLabel.TextSize = 11
	m.readLabel.TextSize = 11
	m.writeLabel.TextSize = 11

	topRow := container.NewHBox(m.statusLabel, m.infoLabel)

	barRow := container.NewGridWithColumns(2,
		container.NewHBox(m.readLabel, m.readBar),
		container.NewHBox(m.writeLabel, m.writeBar),
	)

	scroll := container.NewVScroll(m.channelList)
	scroll.SetMinSize(fyne.NewSize(0, 100))

	go a.monitorLoop(m)

	return container.NewBorder(
		container.NewVBox(topRow, m.totalBar, barRow),
		nil, nil, nil,
		scroll,
	)
}

func (a *App) monitorLoop(m *monitorUI) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		a.refreshMonitor(m)
	}
}

func (a *App) refreshMonitor(m *monitorUI) {
	if a.client == nil || !a.client.IsConnected() {
		m.statusLabel.SetText("Disconnected")
		m.statusLabel.Importance = widget.DangerImportance
		m.infoLabel.Text = ""
		m.infoLabel.Refresh()
		m.totalBar.SetValue(0)
		m.readBar.SetValue(0)
		m.writeBar.SetValue(0)
		m.readLabel.Text = "READ 0%"
		m.readLabel.Refresh()
		m.writeLabel.Text = "WRITE 0%"
		m.writeLabel.Refresh()
		m.channelList.Objects = nil
		m.channelList.Refresh()
		if a.monTotalIn != nil {
			a.monTotalIn.Text = "0 B"
			a.monTotalIn.Refresh()
		}
		if a.monTotalOut != nil {
			a.monTotalOut.Text = "0 B"
			a.monTotalOut.Refresh()
		}
		if a.monChannels != nil {
			a.monChannels.Text = "R0 / W0"
			a.monChannels.Refresh()
		}
		return
	}

	stats := a.client.GetStats()
	if connected, _ := stats["connected"].(bool); !connected {
		m.statusLabel.SetText("Disconnected")
		m.statusLabel.Importance = widget.DangerImportance
		return
	}

	m.statusLabel.SetText("Connected")
	m.statusLabel.Importance = widget.SuccessImportance

	mgrStats, _ := stats["manager_stats"].(map[string]interface{})
	var activeRead, activeWrite int
	if mgrStats != nil {
		activeRead, _ = mgrStats["active_read"].(int)
		activeWrite, _ = mgrStats["active_write"].(int)
		ca, _ := mgrStats["created_at"].(time.Time)
		uptime := ""
		if !ca.IsZero() {
			uptime = fmt.Sprintf(" | Up: %s", fmtDur(time.Since(ca)))
		}
		m.infoLabel.Text = fmt.Sprintf("R%d/W%d ch%s", activeRead, activeWrite, uptime)
		m.infoLabel.Refresh()
	}

	if a.monChannels != nil {
		a.monChannels.Text = fmt.Sprintf("R%d / W%d", activeRead, activeWrite)
		a.monChannels.Refresh()
	}

	chStats, ok := stats["channel_stats"].(map[uint16]channel.Stats)

	var tIn, tOut uint64
	var snapshots []channelSnapshot

	if ok {
		for id, s := range chStats {
			tIn += s.BytesRecv
			tOut += s.BytesSent
			chType := "R"
			if int(id) > activeRead {
				chType = "W"
			}
			snapshots = append(snapshots, channelSnapshot{
				id: id, chType: chType,
				bytesIn: s.BytesRecv, bytesOut: s.BytesSent,
				pktsIn: s.PacketsRecv, pktsOut: s.PacketsSent,
			})
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].id < snapshots[j].id
	})

	m.prevTotal = tIn + tOut
	m.prevTime = time.Now()

	if a.monTotalIn != nil {
		a.monTotalIn.Text = fmtBytes(tIn)
		a.monTotalIn.Refresh()
	}
	if a.monTotalOut != nil {
		a.monTotalOut.Text = fmtBytes(tOut)
		a.monTotalOut.Refresh()
	}

	var readLoad, writeLoad float64
	for _, s := range snapshots {
		total := float64(s.bytesIn + s.bytesOut)
		if s.chType == "R" {
			readLoad += total
		} else {
			writeLoad += total
		}
	}
	totalAll := readLoad + writeLoad
	if totalAll > 0 {
		rPct := readLoad / totalAll
		wPct := writeLoad / totalAll
		m.totalBar.SetValue(1.0)
		m.readBar.SetValue(rPct)
		m.writeBar.SetValue(wPct)
		m.readLabel.Text = fmt.Sprintf("READ %.0f%%", rPct*100)
		m.readLabel.Refresh()
		m.writeLabel.Text = fmt.Sprintf("WRITE %.0f%%", wPct*100)
		m.writeLabel.Refresh()
	} else {
		m.totalBar.SetValue(0)
		m.readBar.SetValue(0)
		m.writeBar.SetValue(0)
	}

	var readSnaps, writeSnaps []channelSnapshot
	for _, snap := range snapshots {
		if snap.chType == "R" {
			readSnaps = append(readSnaps, snap)
		} else {
			writeSnaps = append(writeSnaps, snap)
		}
	}

	m.channelList.Objects = nil

	maxLen := len(readSnaps)
	if len(writeSnaps) > maxLen {
		maxLen = len(writeSnaps)
	}

	for i := 0; i < maxLen; i++ {
		var left, right fyne.CanvasObject
		if i < len(readSnaps) {
			left = a.makeChannelCompact(readSnaps[i], uint64(totalAll), neonCyan)
		} else {
			left = canvas.NewText("", nil)
		}
		if i < len(writeSnaps) {
			right = a.makeChannelCompact(writeSnaps[i], uint64(totalAll), neonMagenta)
		} else {
			right = canvas.NewText("", nil)
		}
		m.channelList.Objects = append(m.channelList.Objects,
			container.NewGridWithColumns(2, left, right))
	}

	m.channelList.Refresh()
}

func (a *App) makeChannelCompact(snap channelSnapshot, totalAll uint64, accent color.Color) fyne.CanvasObject {
	var util float64
	if totalAll > 0 {
		util = float64(snap.bytesIn+snap.bytesOut) / float64(totalAll) * 100
	}

	bar := widget.NewProgressBar()
	bar.SetValue(util / 100)

	idText := canvas.NewText(fmt.Sprintf("#%02d %s", snap.id, snap.chType), accent)
	idText.TextSize = 11
	idText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}

	trafficText := canvas.NewText(
		fmt.Sprintf("IN:%s OUT:%s %d/%d",
			fmtBytes(snap.bytesIn), fmtBytes(snap.bytesOut),
			snap.pktsIn, snap.pktsOut),
		textGrey)
	trafficText.TextSize = 10

	top := container.NewHBox(idText, trafficText)
	return container.NewVBox(top, bar)
}

func fmtDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func fmtBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
