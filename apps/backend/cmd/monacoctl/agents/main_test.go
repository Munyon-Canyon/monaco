package agents

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithSetup(snapshotRepos))
}

var emptyRepo, rootedRepo repoSnapshot

func snapshotRepos() (func(), error) {
	var err error
	if emptyRepo, err = snapshotRepo(); err != nil {
		return nil, err
	}
	rootedRepo, err = snapshotRepo(
		[]string{"commit", "-q", "--allow-empty", "-m", "root"},
		[]string{"branch", "fb-checkpoint-1"},
	)
	return func() {}, err
}
