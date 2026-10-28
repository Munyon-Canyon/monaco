package notify_test

import (
	"flag"
	"os"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const shortChaosSeeds = "5"

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithNATS(), testkit.WithSetup(capShortChaosSeeds))
}

func capShortChaosSeeds() (func(), error) {
	flag.Parse()
	if _, set := os.LookupEnv("CHAOS_SEEDS"); !set && testing.Short() {
		return func() {}, os.Setenv("CHAOS_SEEDS", shortChaosSeeds)
	}
	return func() {}, nil
}
