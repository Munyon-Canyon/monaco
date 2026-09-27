package failing_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m)
}

func TestLeaksAGoroutine(t *testing.T) {
	block := make(chan struct{})
	go func() { <-block }()
}

func failWithDB(t *testing.T) {
	var name string
	if err := testkit.DB(t).QueryRow(context.Background(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	t.Errorf("fixture failure in database=%s", name)
}

func TestFail1(t *testing.T) { failWithDB(t) }
func TestFail2(t *testing.T) { failWithDB(t) }
func TestFail3(t *testing.T) { failWithDB(t) }
func TestFail4(t *testing.T) { failWithDB(t) }
func TestFail5(t *testing.T) { failWithDB(t) }
func TestFail6(t *testing.T) { failWithDB(t) }
