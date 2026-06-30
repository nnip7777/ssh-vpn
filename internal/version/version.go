package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "0.3.1"
	GitCommit = "unknown"
	BuildDate = "unknown"
	AppName   = "ssh-vpn"
)

func String() string {
	return fmt.Sprintf("%s v%s (commit: %s, built: %s, go: %s, os: %s/%s)",
		AppName, Version, GitCommit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func Short() string {
	return fmt.Sprintf("%s v%s", AppName, Version)
}
