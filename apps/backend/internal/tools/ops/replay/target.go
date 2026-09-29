package replay

import (
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const devPort = 54322

func CheckTarget(source, target string) error {
	const op = "replay.CheckTarget"
	var cfgs [2]*pgconn.Config
	for i, url := range []string{source, target} {
		cfg, err := pgconn.ParseConfig(url)
		if err != nil {
			return errs.Wrap(err, errs.CodeInvalidInput, op)
		}
		cfgs[i] = cfg
	}
	s, t := cfgs[0], cfgs[1]
	switch {
	case t.Port == devPort:
		return errs.Wrap(RefusedError("target is on the dev database container monaco-postgres (port 54322)"),
			errs.CodeInvalidInput, op)
	case host(s.Host) == host(t.Host) && s.Port == t.Port && s.Database == t.Database:
		return errs.Wrap(RefusedError("target is the source database"), errs.CodeInvalidInput, op)
	}
	return nil
}

func host(h string) string {
	switch h {
	case "127.0.0.1", "::1":
		return "localhost"
	}
	return h
}
