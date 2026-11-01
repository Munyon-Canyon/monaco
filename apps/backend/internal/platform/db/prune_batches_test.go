package db

import (
	"errors"
	"testing"
)

func TestPruneBatches_pagesUntilAShortBatch(t *testing.T) {
	t.Parallel()
	sizes := []int64{pruneBatch, pruneBatch, 7}
	calls := 0
	deleted, batches, err := pruneBatches(func() (int64, error) {
		n := sizes[calls]
		calls++
		return n, nil
	}, "db.test")
	if err != nil || deleted != 2*pruneBatch+7 || batches != 3 || calls != 3 {
		t.Fatalf("pruneBatches = %d rows in %d batches, %d calls (%v), want %d in 3 batches",
			deleted, batches, calls, err, 2*pruneBatch+7)
	}
}

func TestPruneBatches_stopsOnTheFirstError(t *testing.T) {
	t.Parallel()
	calls := 0
	deleted, batches, err := pruneBatches(func() (int64, error) {
		calls++
		if calls == 2 {
			return 0, errors.New("boom")
		}
		return pruneBatch, nil
	}, "db.test")
	if err == nil || deleted != pruneBatch || batches != 1 {
		t.Fatalf("pruneBatches = %d rows in %d batches (%v), want 1000 in 1 and an error", deleted, batches, err)
	}
}
