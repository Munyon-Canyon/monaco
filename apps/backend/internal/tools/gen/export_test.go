package gen

import (
	"context"
	"fmt"
)

func Apply(root, kind string, args ...string) ([]string, error) {
	g, ok := Find(kind)
	if !ok {
		return nil, fmt.Errorf("no generator %q", kind)
	}
	return g.write(root, args)
}

func Check(what string, err error) { check(what, err) }

func StagingMigrations(dir string) ([]string, error) {
	return gitStagingMigrations(dir)(context.Background())
}

func AtlasHash(dir string) error { return atlasHash(dir)(context.Background()) }
