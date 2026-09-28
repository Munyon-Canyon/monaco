package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/monaco/monaco/apps/backend/internal/platform/lint/nogo"
)

func main() {
	multichecker.Main(nogo.Analyzer(), nogo.TestMainAnalyzer(), nogo.WallclockAnalyzer())
}
