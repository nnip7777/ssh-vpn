package gui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (a *App) createDebugTab() fyne.CanvasObject {
	output := widget.NewMultiLineEntry()
	output.SetText("Click 'Collect' to gather debug info...")
	output.Wrapping = fyne.TextWrapBreak

	scroll := container.NewVScroll(output)

	collectBtn := widget.NewButton("Collect", func() {
		output.SetText("Collecting...")
		info := a.collectDebugInfo()
		output.SetText(info)
	})

	copyBtn := widget.NewButton("Copy to Clipboard", func() {
		info := output.Text
		if info == "" || strings.HasPrefix(info, "Click") {
			return
		}
		if runtime.GOOS == "darwin" {
			cmd := exec.Command("pbcopy")
			cmd.Stdin = strings.NewReader(info)
			cmd.Run()
		}
		dialog.ShowInformation("Copied", "Debug info copied to clipboard!", a.mainWin)
	})

	saveBtn := widget.NewButton("Save to File", func() {
		info := output.Text
		if info == "" || strings.HasPrefix(info, "Click") {
			return
		}
		dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			uc.Write([]byte(info))
			uc.Close()
			dialog.ShowInformation("Saved", "Debug info saved!", a.mainWin)
		}, a.mainWin)
	})

	buttons := container.NewHBox(collectBtn, copyBtn, saveBtn, layout.NewSpacer())

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), buttons),
		nil, nil,
		scroll,
	)
}

func (a *App) collectDebugInfo() string {
	var sb strings.Builder

	sb.WriteString("=== SSH VPN Debug Info ===\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("Version: %s\n", a.GetVersion()))
	sb.WriteString("\n")

	if a.client != nil && a.client.IsConnected() {
		sb.WriteString("Status: CONNECTED\n")
		stats := a.client.GetStats()
		if rc, ok := stats["read_channels"].(int); ok {
			sb.WriteString(fmt.Sprintf("Read channels: %d\n", rc))
		}
		if wc, ok := stats["write_channels"].(int); ok {
			sb.WriteString(fmt.Sprintf("Write channels: %d\n", wc))
		}
		if cs, ok := stats["channel_stats"].(map[uint16]struct {
			BytesSent   uint64
			BytesRecv   uint64
			PacketsSent uint64
			PacketsRecv uint64
		}); ok {
			var totalIn, totalOut uint64
			for _, s := range cs {
				totalIn += s.BytesRecv
				totalOut += s.BytesSent
			}
			sb.WriteString(fmt.Sprintf("Total IN: %d B, Total OUT: %d B\n", totalIn, totalOut))
		}
	} else {
		sb.WriteString("Status: DISCONNECTED\n")
	}
	sb.WriteString("\n")

	sb.WriteString("=== Network Interfaces ===\n")
	sb.WriteString(runCmd("ifconfig", "-a"))
	sb.WriteString("\n")

	sb.WriteString("=== Routing Table ===\n")
	sb.WriteString(runCmd("netstat", "-nr"))
	sb.WriteString("\n")

	sb.WriteString("=== utun5 Interface ===\n")
	sb.WriteString(runCmd("ifconfig", "utun5"))
	sb.WriteString("\n")

	sb.WriteString("=== Ping 10.8.0.1 ===\n")
	sb.WriteString(runCmd("ping", "-c", "3", "-W", "2000", "10.8.0.1"))
	sb.WriteString("\n")

	sb.WriteString("=== Ping 192.168.0.1 ===\n")
	sb.WriteString(runCmd("ping", "-c", "2", "-W", "2000", "192.168.0.1"))
	sb.WriteString("\n")

	sb.WriteString("=== Server Port Test ===\n")
	sb.WriteString(runCmd("nc", "-z", "-w", "3", "72.56.246.125", "2222"))
	sb.WriteString("\n")

	return sb.String()
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		result += fmt.Sprintf("[exit: %v]\n", err)
	}
	if len(result) > 4000 {
		result = result[:4000] + "\n... (truncated)\n"
	}
	return result
}
