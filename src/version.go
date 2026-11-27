package main

import (
	"strconv"
	"time"
)

// Build information is injected by the Makefile and CI linker flags. The build
// date is derived at startup from the single embedded Unix build epoch.
var (
	Version      = "devel"
	CommitID     = "N/A"
	BuildDate    = "N/A"
	BuildEpoch   = "0"
	OfficialSite = ""
)

func buildEpoch() int64 {
	epoch, err := strconv.ParseInt(BuildEpoch, 10, 64)
	if err != nil {
		return 0
	}
	return epoch
}

func init() {
	if epoch := buildEpoch(); epoch > 0 {
		BuildDate = buildDateFromEpoch(epoch)
	}
}

func buildDateFromEpoch(epoch int64) string {
	if epoch <= 0 {
		return "N/A"
	}
	return time.Unix(epoch, 0).UTC().Format("2006-01-02T15:04:05Z")
}
