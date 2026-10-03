package feed_test

import (
	"flag"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var update = flag.Bool("update", false, "rewrite testdata/render.golden from the cases")

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB())
}
