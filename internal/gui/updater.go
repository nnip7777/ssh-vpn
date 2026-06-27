package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"go.uber.org/zap"
)

type Updater struct {
	currentVersion string
	logger         *zap.Logger
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func NewUpdater(version string, logger *zap.Logger) *Updater {
	return &Updater{
		currentVersion: version,
		logger:         logger,
	}
}

func (u *Updater) CheckForUpdates(window fyne.Window) {
	go func() {
		release, err := u.fetchLatestRelease()
		if err != nil {
			u.logger.Warn("failed to check for updates", zap.Error(err))
			dialog.ShowInformation("Update Check", "Unable to check for updates", window)
			return
		}

		if release.TagName != u.currentVersion {
			dialog.ShowConfirm("Update Available",
				fmt.Sprintf("New version %s is available.\nCurrent version: %s\n\nOpen download page?",
					release.TagName, u.currentVersion),
				func(ok bool) {
			if ok {
					u, _ := url.Parse(release.HTMLURL)
					if u != nil {
						fyne.CurrentApp().OpenURL(u)
					}
				}
				}, window)
		} else {
			dialog.ShowInformation("Update Check", "You are running the latest version", window)
		}
	}()
}

func (u *Updater) fetchLatestRelease() (*GitHubRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/nnip7777/ssh-vpn/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}
