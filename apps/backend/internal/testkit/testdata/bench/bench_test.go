package bench_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const tests = 64

func TestMain(m *testing.M) {
	testkit.Main(m)
}

func TestCloneBench(t *testing.T) {
	t.Run("warm", func(t *testing.T) { testkit.DB(t) })
	start := time.Now()
	t.Run("clones", func(t *testing.T) {
		for i := range tests {
			t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
				t.Parallel()
				db := testkit.DB(t)
				for range 5 {
					if _, err := db.Exec(context.Background(), "SELECT count(*) FROM events"); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	})
	t.Logf("bench tests=%d phase_ns=%d", tests, time.Since(start).Nanoseconds())
}
