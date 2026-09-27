package migrations

import (
	"embed"
	"strings"
)

//go:embed *.sql
var files embed.FS

func Latest() string {
	entries, _ := files.ReadDir(".")
	latest := ""
	for _, e := range entries {
		version, _, _ := strings.Cut(e.Name(), "_")
		latest = max(latest, version)
	}
	return latest
}
