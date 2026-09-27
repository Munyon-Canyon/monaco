package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/monaco/monaco/apps/backend/internal/platform/lint/nogo"
)

func main() {
	singlechecker.Main(nogo.Analyzer())
}
