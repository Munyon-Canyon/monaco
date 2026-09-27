package testkit_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMainWithNATS_givesATestBothADatabaseAndItsOwnStreams(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	b := testkit.NATS(t)

	var one int
	if err := db.QueryRow(t.Context(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("SELECT 1 = %d, %v", one, err)
	}
	if err := b.Conn.VerifyStreams(t.Context()); err != nil {
		t.Fatalf("VerifyStreams on the test's streams = %v", err)
	}
}
