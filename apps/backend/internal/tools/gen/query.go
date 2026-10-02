package gen

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	sqlcConfig = "sqlc.yaml"
	sqlcBegin  = "# BEGIN GENERATED modules"
	sqlcEnd    = "# END GENERATED modules"
	sqlcEntry  = "- {engine: postgresql, schema: migrations, queries: queries/%[1]s, gen: {go: {<<: *common, package: sqlc, out: internal/modules/%[1]s/sqlc, output_db_file_name: db.gen.go, output_models_file_name: models.gen.go, output_files_suffix: .gen}}}\n"
)

func planQuery(root *os.Root, modPath string, args []string) (Plan, error) {
	module, name := args[0], args[1]
	if err := requireModule(root, module, name, exportPattern); err != nil {
		return Plan{}, err
	}
	d := newData(modPath, module, name)
	return Plan{Create: renderAll(d, map[string]string{
		filepath.Join("queries", module, d.File+".sql"):             "query/query.sql.tmpl",
		filepath.Join(moduleDir(module), "app", d.File+"_query.go"): "query/query.go.tmpl",
	})}, nil
}

func regenerateQueries(ctx context.Context, dir string) error {
	if err := Regenerate(ctx, dir); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "sqlc", "generate")
	if _, err := os.Stat(filepath.Join(dir, "..", "..", ".bin", "sqlc")); err == nil {
		cmd = exec.CommandContext(ctx, "../../.bin/sqlc", "generate")
	}
	cmd.Dir = dir
	return runQuiet(cmd, "gen.regenerateQueries")
}

func planSqlc(*os.Root, string, []string) (Plan, error) { return Plan{}, nil }

func syncSqlc(_ context.Context, dir string) error {
	const op = "gen.syncSqlc"
	root, err := os.OpenRoot(dir)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	defer func() { _ = root.Close() }()
	modules, err := queryModules(root)
	if err != nil {
		return err
	}
	cfg, err := root.ReadFile(sqlcConfig)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	lines := strings.SplitAfter(string(cfg), "\n")
	begin := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == sqlcBegin })
	end := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == sqlcEnd })
	if begin < 0 || end < begin {
		return invalid(op, "%s needs a %q line followed by a %q line", sqlcConfig, sqlcBegin, sqlcEnd)
	}
	indent := lines[begin][:len(lines[begin])-len(strings.TrimLeft(lines[begin], " "))]
	var block strings.Builder
	for _, m := range modules {
		for line := range strings.Lines(strings.ReplaceAll(sqlcEntry, "%[1]s", m)) {
			block.WriteString(indent + line)
		}
	}
	out := strings.Join(lines[:begin+1], "") + block.String() + strings.Join(lines[end:], "")
	if out == string(cfg) {
		return nil
	}
	return writeFile(root, sqlcConfig, out)
}

func queryModules(root *os.Root) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), "internal/modules")
	if err != nil && !os.IsNotExist(err) {
		return nil, errs.Wrap(err, errs.CodeInternal, "gen.queryModules")
	}
	var out []string
	for _, e := range entries {
		sqls, _ := fs.Glob(root.FS(), "queries/"+e.Name()+"/*.sql")
		if e.IsDir() && len(sqls) > 0 {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
