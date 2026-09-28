package nodb

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithSetup(setup))
}

func setup() (func(), error) {
	if os.Getenv("NODB_FAIL_SETUP") == "1" {
		return nil, errors.New("setup refused")
	}
	fmt.Println("fixture: setup")
	return func() { fmt.Println("fixture: cleanup") }, nil
}

func TestRuns(t *testing.T) {
	fmt.Println("fixture: test")
}
