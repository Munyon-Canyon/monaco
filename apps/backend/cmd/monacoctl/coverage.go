package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	coverageUsage   = "usage: monacoctl coverage --profile cover.out [--covdir dir]..."
	coverageExclude = "coverage.exclude"
)

type coverBlock struct {
	file               string
	startLine, endLine int
	stmts              int
	hit                bool
}

type coverage map[string]coverBlock

type dirList []string

func (d *dirList) String() string { return strings.Join(*d, ",") }

func (d *dirList) Set(v string) error {
	*d = append(*d, v)
	return nil
}

type coverageEnv struct {
	moduleDir, goBin, tmpDir string
}

func (env coverageEnv) run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	profile := fs.String("profile", "", "")
	var covdirs dirList
	fs.Var(&covdirs, "covdir", "")
	if fs.Parse(args) != nil || *profile == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, coverageUsage)
		return 2
	}
	profiles := []string{*profile}
	if len(covdirs) > 0 {
		text, err := env.covdataText(context.Background(), covdirs)
		defer func() { _ = os.Remove(text) }()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl coverage: %v\n", err)
			return 1
		}
		profiles = append(profiles, text)
	}
	missed, err := checkCoverage(env.moduleDir, profiles, stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl coverage: %v\n", err)
		return 1
	}
	if missed > 0 {
		_, _ = fmt.Fprintf(stderr, "monacoctl coverage: %d statements uncovered, the gate is 100%%\n", missed)
		return 1
	}
	return 0
}

func (env coverageEnv) covdataText(ctx context.Context, dirs []string) (string, error) {
	const op = "monacoctl.covdataText"
	file, err := os.CreateTemp(env.tmpDir, "covdata-*.out")
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, op)
	}
	_ = file.Close()
	cmd := exec.CommandContext(
		ctx,
		env.goBin,
		"tool",
		"covdata",
		"textfmt",
		"-i="+strings.Join(dirs, ","),
		"-o="+file.Name(),
	)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return file.Name(), errs.Wrap(fmt.Errorf("%w: %s", err, bytes.TrimSpace(msg)), errs.CodeInternal, op)
	}
	return file.Name(), nil
}

func checkCoverage(moduleDir string, profiles []string, stdout io.Writer) (int, error) {
	const op = "monacoctl.checkCoverage"
	module, err := modulePath(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return 0, err
	}
	exclude, err := os.ReadFile(filepath.Join(moduleDir, coverageExclude))
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInternal, op)
	}
	cov := coverage{}
	for _, p := range profiles {
		if err := cov.readFile(p); err != nil {
			return 0, err
		}
	}
	return cov.report(module, strings.Fields(string(exclude)), stdout), nil
}

func modulePath(goMod string) (string, error) {
	data, err := os.ReadFile(goMod)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, "monacoctl.modulePath")
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", errs.New(errs.CodeInternal, "monacoctl.modulePath", slog.String("missing_module_line", goMod))
}

func (c coverage) readFile(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "monacoctl.readProfile")
	}
	defer func() { _ = file.Close() }()
	return c.read(file)
}

func (c coverage) read(r io.Reader) error {
	const op = "monacoctl.readProfile"
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode: ") {
			continue
		}
		b, key, err := parseBlock(line)
		if err != nil {
			return err
		}
		prev, seen := c[key]
		b.hit = b.hit || seen && prev.hit
		c[key] = b
	}
	if err := scanner.Err(); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return nil
}

func parseBlock(line string) (coverBlock, string, error) {
	const op = "monacoctl.parseBlock"
	key, counts, ok := strings.Cut(line, " ")
	stmts, count, ok2 := strings.Cut(counts, " ")
	file, span, ok3 := strings.Cut(key, ":")
	start, end, ok4 := strings.Cut(span, ",")
	if !ok || !ok2 || !ok3 || !ok4 {
		return coverBlock{}, "", errs.New(errs.CodeDecodeFailed, op, slog.String("line", line))
	}
	b := coverBlock{file: file}
	var err error
	for _, f := range []struct {
		dst *int
		src string
	}{
		{&b.startLine, strings.Split(start, ".")[0]},
		{&b.endLine, strings.Split(end, ".")[0]},
		{&b.stmts, stmts},
	} {
		if *f.dst, err = strconv.Atoi(f.src); err != nil {
			return coverBlock{}, "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("line", line))
		}
	}
	n, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return coverBlock{}, "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("line", line))
	}
	b.hit = n > 0
	return b, key, nil
}

func excluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "/") && strings.HasPrefix(rel, p) {
			return true
		}
		if ok, _ := path.Match(p, path.Base(rel)); ok && !strings.Contains(p, "/") {
			return true
		}
	}
	return false
}

func (c coverage) report(module string, exclude []string, w io.Writer) int {
	var missed []coverBlock
	total, uncovered := 0, 0
	for _, b := range c {
		b.file = strings.TrimPrefix(b.file, module+"/")
		if excluded(b.file, exclude) {
			continue
		}
		total += b.stmts
		if !b.hit {
			uncovered += b.stmts
			missed = append(missed, b)
		}
	}
	slices.SortFunc(missed, func(a, b coverBlock) int {
		if c := strings.Compare(a.file, b.file); c != 0 {
			return c
		}
		return a.startLine - b.startLine
	})
	for _, b := range missed {
		_, _ = fmt.Fprintf(w, "%s:%d-%d: %d statements not covered\n", b.file, b.startLine, b.endLine, b.stmts)
	}
	pct := 100.0
	if total > 0 {
		pct = 100 * float64(total-uncovered) / float64(total)
	}
	_, _ = fmt.Fprintf(w, "coverage: %.2f%% of %d statements\n", pct, total)
	return uncovered
}
