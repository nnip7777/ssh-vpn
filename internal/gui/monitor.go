package gui

import (
	"fmt"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/nnip7777/ssh-vpn/internal/channel"
)

type monitorUI struct {
	statusLabel     *widget.Label
	uptimeLabel     *widget.Label
	totalInLabel    *widget.Label
	totalOutLabel   *widget.Label
	packetsInLabel  *widget.Label
	packetsOutLabel *widget.Label
	chCreatedLabel  *widget.Label
	chClosedLabel   *widget.Label
	chActiveLabel   *widget.Label
	balanceLabel    *widget.Label
	channelList     *fyne.Container
	prevTotal       uint64
	prevTime        time.Time
}

type channelSnapshot struct {
	id        uint16
	chType    string
	util      float64
	bytesIn   uint64
	bytesOut  uint64
	pktsIn    uint64
	pktsOut   uint64
}

func (a *App) createMonitorTab() fyne.CanvasObject {
	title := widget.NewLabel("VPN Monitor")
	title.TextStyle = fyne.TextStyle{Bold: true}

	m := &monitorUI{
		statusLabel:     widget.NewLabel("Disconnected"),
		uptimeLabel:     widget.NewLabel("Uptime: -"),
		totalInLabel:    widget.NewLabel("IN: 0 B/s"),
		totalOutLabel:   widget.NewLabel("OUT: 0 B/s"),
		packetsInLabel:  widget.NewLabel("Pkts IN: 0"),
		packetsOutLabel: widget.NewLabel("Pkts OUT: 0"),
		chCreatedLabel:  widget.NewLabel("Created: 0"),
		chClosedLabel:   widget.NewLabel("Closed: 0"),
		chActiveLabel:   widget.NewLabel("Active: R0 / W0"),
		balanceLabel:    widget.NewLabel("Balance: -"),
		channelList:     container.NewVBox(),
		prevTime:        time.Now(),
	}

	a.monUI = m
	m.statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	info := container.NewGridWithColumns(3,
		container.NewVBox(m.statusLabel, m.uptimeLabel),
		container.NewVBox(m.chActiveLabel, m.chCreatedLabel, m.chClosedLabel, m.balanceLabel),
		container.NewVBox(m.totalInLabel, m.totalOutLabel, m.packetsInLabel, m.packetsOutLabel),
	)

	scroll := container.NewVScroll(m.channelList)

	go a.monitorLoop(m)

	return container.NewBorder(
		container.NewVBox(title, widget.NewSeparator(), info, widget.NewSeparator()),
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
		m.uptimeLabel.SetText("Uptime: -")
		m.chActiveLabel.SetText("Active: R0 / W0")
		m.balanceLabel.SetText("Balance: -")
		m.channelList.Objects = nil
		m.channelList.Refresh()
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
	var totalCreated, totalClosed uint64
	if mgrStats != nil {
		activeRead, _ = mgrStats["active_read"].(int)
		activeWrite, _ = mgrStats["active_write"].(int)
		totalCreated, _ = mgrStats["total_created"].(uint64)
		totalClosed, _ = mgrStats["total_closed"].(uint64)
		ca, _ := mgrStats["created_at"].(time.Time)
		m.chActiveLabel.SetText(fmt.Sprintf("Active: R%d / W%d", activeRead, activeWrite))
		m.chCreatedLabel.SetText(fmt.Sprintf("Created: %d", totalCreated))
		m.chClosedLabel.SetText(fmt.Sprintf("Closed: %d", totalClosed))
		if !ca.IsZero() {
			m.uptimeLabel.SetText(fmt.Sprintf("Uptime: %s", fmtDur(time.Since(ca))))
		}
	}

	chStats, ok := stats["channel_stats"].(map[uint16]channel.Stats)

	var tIn, tOut, pIn, pOut uint64
	var snapshots []channelSnapshot

	if ok {
		for id, s := range chStats {
			tIn += s.BytesRecv
			tOut += s.BytesSent
			pIn += s.PacketsRecv
			pOut += s.PacketsSent

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

	elapsed := time.Since(m.prevTime).Seconds()
	var throughput float64
	if elapsed > 0 && m.prevTotal > 0 {
		throughput = float64(tIn+tOut-m.prevTotal) / elapsed
	}
	m.prevTotal = tIn + tOut
	m.prevTime = time.Now()

	m.totalInLabel.SetText(fmt.Sprintf("IN: %s", fmtBytes(tIn)))
	m.totalOutLabel.SetText(fmt.Sprintf("OUT: %s", fmtBytes(tOut)))
	m.packetsInLabel.SetText(fmt.Sprintf("Pkts IN: %d", pIn))
	m.packetsOutLabel.SetText(fmt.Sprintf("Pkts OUT: %d", pOut))

	if len(snapshots) > 0 && throughput > 0 {
		readLoad := 0.0
		writeLoad := 0.0
		for _, s := range snapshots {
			total := float64(s.bytesIn + s.bytesOut)
			if tIn+tOut > 0 {
				if s.chType == "R" {
					readLoad += total / float64(tIn+tOut) * 100
				} else {
					writeLoad += total / float64(tIn+tOut) * 100
				}
			}
		}
		m.balanceLabel.SetText(fmt.Sprintf("Load: R%.0f%% W%.0f%% @ %s/s", readLoad, writeLoad, fmtBytes(uint64(throughput))))
	} else {
		m.balanceLabel.SetText("Load: idle")
	}

	m.channelList.Objects = nil
	for _, snap := range snapshots {
		totalAll := tIn + tOut
		var util float64
		if totalAll > 0 {
			util = float64(snap.bytesIn+snap.bytesOut) / float64(totalAll) * 100
		}

		bar := widget.NewProgressBar()
		bar.SetValue(util / 100)

		label := widget.NewLabel(fmt.Sprintf(
			"#%02d %s  %s%%  IN:%s  OUT:%s  %d/%d pkts",
			snap.id, snap.chType, fmt.Sprintf("%.0f", util),
			fmtBytes(snap.bytesIn), fmtBytes(snap.bytesOut),
			snap.pktsIn, snap.pktsOut,
		))

		m.channelList.Objects = append(m.channelList.Objects,
			container.NewBorder(nil, nil, nil, nil,
				container.NewVBox(bar, label)))
	}

	m.channelList.Refresh()
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
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
