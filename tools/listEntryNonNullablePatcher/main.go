package main

import (
	"github.com/samsarahq/thunder/tools/listEntryNonNullablePatcher/listEntryNonNullablePatcher"

	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(listEntryNonNullablePatcher.Analyzer)
}
