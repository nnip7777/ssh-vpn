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

const sparklineLen = 40

type monitorUI struct {
	readBar     *widget.ProgressBar
	writeBar    *widget.ProgressBar
	readLabel   *canvas.Text
	writeLabel  *canvas.Text
	channelList *fyne.Container
	sparkLine   *SparkLine
	sparkLabel  *canvas.Text
	statsLabel  *canvas.Text
	prevTotal   uint64
	prevTime    time.Time
	sparkData   [sparklineLen]float64
	sparkIdx    int
}

type channelSnapshot struct {
	id       uint16
	chType   string
	bytesIn  uint64
	bytesOut uint64
	pktsIn   uint64
	pktsOut  uint64
	errors   uint64
}

func (a *App) createMonitorTab() fyne.CanvasObject {
	m := &monitorUI{
		readBar:     widget.NewProgressBar(),
		writeBar:    widget.NewProgressBar(),
		readLabel:   canvas.NewText("READ 0%", neonCyan),
		writeLabel:  canvas.NewText("WRITE 0%", neonMagenta),
		channelList: container.NewVBox(),
		sparkLine:   NewSparkLine(neonCyan, sparklineLen),
		sparkLabel:  canvas.NewText("0 B/s", neonGreen),
		statsLabel:  canvas.NewText("ERR:0  RETR:0  DROP_IN:0  DROP_OUT:0", neonGreen),
		prevTime:    time.Now(),
	}

	a.monUI = m
	m.readLabel.TextSize = 10
	m.writeLabel.TextSize = 10
	m.readLabel.TextStyle = fyne.TextStyle{Bold: true}
	m.writeLabel.TextStyle = fyne.TextStyle{Bold: true}
	m.sparkLabel.TextSize = 12
	m.sparkLabel.TextStyle = fyne.TextStyle{Bold: true}
	m.statsLabel.TextSize = 11
	m.statsLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}

	barRow := container.NewGridWithColumns(2,
		container.NewHBox(m.readLabel, m.readBar),
		container.NewHBox(m.writeLabel, m.writeBar),
	)

	sparkRow := container.NewBorder(nil, nil, m.sparkLabel, nil, m.sparkLine)

	go a.monitorLoop(m)

	return container.NewBorder(
		container.NewVBox(barRow, sparkRow, m.statsLabel),
		nil, nil, nil,
		m.channelList,
	)
}

func (a *App) monitorLoop(m *monitorUI) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.monStopCh:
			return
		case <-ticker.C:
			a.refreshMonitor(m)
		}
	}
}

func (a *App) refreshMonitor(m *monitorUI) {
	c := a.client
	if c == nil || !c.IsConnected() {
		m.readBar.SetValue(0)
		m.writeBar.SetValue(0)
		m.readLabel.Text = "READ 0%"
		m.readLabel.Refresh()
		m.writeLabel.Text = "WRITE 0%"
		m.writeLabel.Refresh()
		m.sparkLabel.Text = "0 B/s"
		m.sparkLabel.Refresh()
		m.statsLabel.Text = ""
		m.statsLabel.Refresh()
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

	stats := c.GetStats()
	if connected, _ := stats["connected"].(bool); !connected {
		return
	}

	mgrStats, _ := stats["manager_stats"].(map[string]interface{})
	var activeRead, activeWrite int
	var totalCreated, totalClosed uint64
	if mgrStats != nil {
		activeRead, _ = mgrStats["active_read"].(int)
		activeWrite, _ = mgrStats["active_write"].(int)
		totalCreated, _ = mgrStats["total_created"].(uint64)
		totalClosed, _ = mgrStats["total_closed"].(uint64)
	}

	if a.monChannels != nil {
		lifecycle := ""
		if totalCreated > 0 || totalClosed > 0 {
			lifecycle = fmt.Sprintf(" (+%d/-%d)", totalCreated, totalClosed)
		}
		a.monChannels.Text = fmt.Sprintf("R%d / W%d%s", activeRead, activeWrite, lifecycle)
		a.monChannels.Refresh()
	}

	chStats, ok := stats["channel_stats"].(map[uint16]channel.Stats)

	var tIn, tOut uint64
	var totalErrors, totalRetransmits uint64
	var snapshots []channelSnapshot

	if ok {
		for id, s := range chStats {
			tIn += s.BytesRecv
			tOut += s.BytesSent
			totalErrors += s.Errors
			totalRetransmits += s.Retransmits
			chType := "R"
			if s.Type == channel.ChannelWrite {
				chType = "W"
			}
			snapshots = append(snapshots, channelSnapshot{
				id: id, chType: chType,
				bytesIn: s.BytesRecv, bytesOut: s.BytesSent,
				pktsIn: s.PacketsRecv, pktsOut: s.PacketsSent,
				errors: s.Errors,
			})
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].id < snapshots[j].id
	})

	now := time.Now()
	elapsed := now.Sub(m.prevTime).Seconds()
	if elapsed < 0.5 {
		elapsed = 1.0
	}
	currentTotal := tIn + tOut
	delta := currentTotal - m.prevTotal
	if currentTotal < m.prevTotal {
		delta = 0
	}
	throughput := float64(delta) / elapsed
	m.prevTotal = currentTotal
	m.prevTime = now

	idx := m.sparkIdx % sparklineLen
	m.sparkData[idx] = throughput
	m.sparkIdx++

	var maxSpark float64
	for _, v := range m.sparkData {
		if v > maxSpark {
			maxSpark = v
		}
	}
	m.sparkLine.Push(throughput)
	m.sparkLine.SetMax(maxSpark)

	m.sparkLabel.Text = fmtThroughput(throughput)
	m.sparkLabel.Refresh()

	dropIn, _ := stats["dropped_in"].(uint64)
	dropOut, _ := stats["dropped_out"].(uint64)
	m.statsLabel.Text = fmt.Sprintf("ERR:%d  RETR:%d  DROP_IN:%d  DROP_OUT:%d",
		totalErrors, totalRetransmits, dropIn, dropOut)
	m.statsLabel.Refresh()

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
		m.readBar.SetValue(rPct)
		m.writeBar.SetValue(wPct)
		m.readLabel.Text = fmt.Sprintf("READ %.0f%%", rPct*100)
		m.readLabel.Refresh()
		m.writeLabel.Text = fmt.Sprintf("WRITE %.0f%%", wPct*100)
		m.writeLabel.Refresh()
	} else {
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

func fmtThroughput(bps float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bps >= GB:
		return fmt.Sprintf("%.1f GB/s", bps/float64(GB))
	case bps >= MB:
		return fmt.Sprintf("%.1f MB/s", bps/float64(MB))
	case bps >= KB:
		return fmt.Sprintf("%.1f KB/s", bps/float64(KB))
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}

func (a *App) makeChannelCompact(snap channelSnapshot, totalAll uint64, accent color.Color) fyne.CanvasObject {
	var util float64
	if totalAll > 0 {
		util = float64(snap.bytesIn+snap.bytesOut) / float64(totalAll) * 100
	}

	bar := widget.NewProgressBar()
	bar.SetValue(util / 100)

	idText := canvas.NewText(fmt.Sprintf("#%02d %s", snap.id, snap.chType), accent)
	idText.TextSize = 10
	idText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}

	errStr := ""
	if snap.errors > 0 {
		errStr = fmt.Sprintf(" E:%d", snap.errors)
	}
	trafficText := canvas.NewText(
		fmt.Sprintf("IN:%s OUT:%s %d/%d%s",
			fmtBytes(snap.bytesIn), fmtBytes(snap.bytesOut),
			snap.pktsIn, snap.pktsOut, errStr),
		textGrey)
	trafficText.TextSize = 9

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
