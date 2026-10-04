package garden

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const Deadcode = "golang.org/x/tools/cmd/deadcode@v0.50.0"

type Kind string

const (
	KindLint     Kind = "Candidate lint"
	KindDeadCode Kind = "Dead code"
	KindMutants  Kind = "Surviving mutants"
	KindDrift    Kind = "Generator drift"
)

type Finding struct {
	Rule string
	File string
	Line int
	Text string
}

type Section struct {
	Kind     Kind
	Findings []Finding
	Skipped  bool
	Err      error
}

type Report struct {
	Sections []Section
}

type Result struct {
	Stdout []byte
	Stderr string
	Code   int
}

type Exec func(ctx context.Context, dir, name string, args ...string) (Result, error)

type Config struct {
	ModuleDir    string
	GolangciLint string
	TempDir      string
	Exec         Exec
	Mutants      func(ctx context.Context) ([]Finding, error)
}

type session struct {
	Config
	top, prefix string
}

type check struct {
	kind Kind
	run  func(ctx context.Context) ([]Finding, error)
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	s := session{Config: cfg}
	out, err := s.run(ctx, cfg.ModuleDir, "git", "rev-parse", "--show-toplevel", "--show-prefix")
	if err != nil {
		return Report{}, err
	}
	lines := strings.SplitN(string(out), "\n", 3)
	if len(lines) < 2 {
		return Report{}, errs.New(errs.CodeDecodeFailed, "garden.Run")
	}
	s.top, s.prefix = lines[0], lines[1]
	var report Report
	for _, c := range s.checks() {
		section := Section{Kind: c.kind, Skipped: c.run == nil}
		if !section.Skipped {
			section.Findings, section.Err = c.run(ctx)
		}
		report.Sections = append(report.Sections, section)
	}
	return report, nil
}

func (r Report) Failed() []Section {
	var failed []Section
	for _, s := range r.Sections {
		if s.Err != nil {
			failed = append(failed, s)
		}
	}
	return failed
}

func (s session) checks() []check {
	var mutants func(context.Context) ([]Finding, error)
	if s.Mutants != nil {
		mutants = s.mutants
	}
	return []check{
		{KindDeadCode, s.deadcode},
		{KindLint, s.lint},
		{KindMutants, mutants},
		{KindDrift, s.drift},
	}
}

type toolError struct {
	cmd    string
	code   int
	stderr string
}

func (e toolError) Error() string {
	return fmt.Sprintf("%s exited %d: %s", e.cmd, e.code, strings.TrimSpace(e.stderr))
}

type dirtyTreeError []string

func (e dirtyTreeError) Error() string {
	return "the drift check needs a clean tree; commit or stash: " + strings.Join(e, ", ")
}

func (s session) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return s.runAllowing(ctx, 0, dir, name, args...)
}

func (s session) runAllowing(ctx context.Context, okCode int, dir, name string, args ...string) ([]byte, error) {
	res, err := s.Exec(ctx, dir, name, args...)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "garden.run")
	}
	if res.Code != 0 && res.Code != okCode {
		return nil, toolError{
			cmd:    strings.Join(append([]string{name}, args...), " "),
			code:   res.Code,
			stderr: res.Stderr,
		}
	}
	return res.Stdout, nil
}

func (s session) repoPath(file string) string {
	if filepath.IsAbs(file) {
		if rel, err := filepath.Rel(s.top, file); err == nil {
			return filepath.ToSlash(rel)
		}
		return file
	}
	return path.Join(s.prefix, filepath.ToSlash(file))
}

type golangciOutput struct {
	Issues []struct {
		FromLinter string `json:"FromLinter"`
		Text       string `json:"Text"`
		Pos        struct {
			Filename string `json:"Filename"`
			Line     int    `json:"Line"`
		} `json:"Pos"`
	} `json:"Issues"`
}

func (s session) lint(ctx context.Context) ([]Finding, error) {
	const op = "garden.lint"
	config, err := CandidateConfig(s.ModuleDir)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(s.TempDir, "golangci-candidate-*.yml")
	if err == nil {
		defer func() { _ = os.Remove(file.Name()) }()
		_, err = file.Write(config)
		err = errors.Join(err, file.Close())
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out, err := s.runAllowing(
		ctx,
		1,
		s.ModuleDir,
		s.GolangciLint,
		"run",
		"--allow-parallel-runners",
		"--config",
		file.Name(),
		"--output.json.path",
		"stdout",
		"--show-stats=false",
		"./...",
	)
	if err != nil {
		return nil, err
	}
	var parsed golangciOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	findings := make([]Finding, 0, len(parsed.Issues))
	for _, i := range parsed.Issues {
		findings = append(
			findings,
			Finding{Rule: i.FromLinter, File: s.repoPath(i.Pos.Filename), Line: i.Pos.Line, Text: i.Text},
		)
	}
	return findings, nil
}

type deadcodePackage struct {
	Funcs []struct {
		Name     string `json:"Name"`
		Position struct {
			File string `json:"File"`
			Line int    `json:"Line"`
		} `json:"Position"`
		Generated bool `json:"Generated"`
	} `json:"Funcs"`
}

func (s session) deadcode(ctx context.Context) ([]Finding, error) {
	out, err := s.run(ctx, s.ModuleDir, "go", "run", Deadcode, "-test", "-json", "./...")
	if err != nil {
		return nil, err
	}
	var pkgs []deadcodePackage
	if err := json.Unmarshal(out, &pkgs); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, "garden.deadcode")
	}
	var findings []Finding
	for _, p := range pkgs {
		for _, f := range p.Funcs {
			if f.Generated {
				continue
			}
			findings = append(findings, Finding{
				File: s.repoPath(f.Position.File),
				Line: f.Position.Line,
				Text: "unreachable func " + f.Name,
			})
		}
	}
	return findings, nil
}

func (s session) mutants(ctx context.Context) ([]Finding, error) {
	findings, err := s.Mutants(ctx)
	for i := range findings {
		findings[i].File = s.repoPath(findings[i].File)
	}
	return findings, err
}

func (s session) generators() [][]string {
	return [][]string{{"go", "generate", "./..."}}
}

func (s session) drift(ctx context.Context) ([]Finding, error) {
	dirty, err := s.status(ctx)
	if err != nil {
		return nil, err
	}
	if len(dirty) > 0 {
		return nil, dirtyTreeError(dirty)
	}
	for _, g := range s.generators() {
		if _, err := s.run(ctx, s.ModuleDir, g[0], g[1:]...); err != nil {
			return nil, err
		}
	}
	changed, err := s.status(ctx)
	if err != nil {
		return nil, err
	}
	lines, err := s.firstChangedLines(ctx)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(changed))
	for _, entry := range changed {
		code, file := strings.TrimSpace(entry[:2]), entry[3:]
		findings = append(findings, Finding{File: file, Line: max(lines[file], 1), Text: driftText(code)})
	}
	return findings, nil
}

func driftText(code string) string {
	switch code {
	case "??":
		return "a generator writes this file and it is not committed"
	case "D":
		return "a generator deletes this committed file"
	default:
		return "the committed file differs from what the generators write"
	}
}

func (s session) status(ctx context.Context) ([]string, error) {
	out, err := s.run(ctx, s.ModuleDir, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var entries []string
	for line := range strings.Lines(string(out)) {
		if line = strings.TrimRight(line, "\n"); len(line) > 3 {
			entries = append(entries, line)
		}
	}
	return entries, nil
}

func (s session) firstChangedLines(ctx context.Context) (map[string]int, error) {
	out, err := s.run(ctx, s.ModuleDir, "git", "diff", "-U0", "--no-color", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	lines := map[string]int{}
	file := ""
	for line := range strings.Lines(string(out)) {
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			file = strings.TrimSpace(strings.TrimPrefix(line, "+++ b/"))
		case strings.HasPrefix(line, "@@ ") && file != "" && lines[file] == 0:
			fields := strings.Fields(line)
			if len(fields) > 2 {
				start, _, _ := strings.Cut(strings.TrimPrefix(fields[2], "+"), ",")
				lines[file], _ = strconv.Atoi(start)
			}
		}
	}
	return lines, nil
}
