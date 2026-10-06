package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	mergeSpecDir             = "apps/backend/api/spec"
	mergeBundle              = "apps/backend/api/openapi.yaml"
	mergeAPIDir              = "apps/backend/internal/platform/httpx/api"
	mergeBaseGo              = mergeAPIDir + "/api.gen.go"
	mergeRootDir             = "../../../.."
	longProbeError errs.Code = "ranking_probe_with_a_name_longer_than_any_other_error_code"

	referralProbeError errs.Code = "referral_probe"
)

func codesWith(extra ...errs.Code) []errs.Code {
	return slices.Sorted(slices.Values(append(errs.All(), extra...)))
}

type mergeRepo struct {
	t   *testing.T
	dir string
}

func (r mergeRepo) generate(codes []errs.Code) {
	r.t.Helper()
	specs := filepath.Join(r.dir, mergeSpecDir)
	bundle, err := bundleOpenAPI(specs, codes)
	if err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, mergeBundle), bundle, 0o600); err != nil {
		r.t.Fatal(err)
	}
	if err := genAPIsLocked(specs, filepath.Join(r.dir, mergeAPIDir), backendModule+"/internal/platform/httpx/api",
		codes...); err != nil {
		r.t.Fatal(err)
	}
}

func (r mergeRepo) addRoute(module, op string) {
	r.t.Helper()
	file := filepath.Join(r.dir, mergeSpecDir, module+".yaml")
	body := readText(r.t, file)
	route := "  /v1/probe/" + op + ":\n    get:\n      operationId: " + op + "\n      tags: [" + module + "]\n" +
		"      responses:\n        \"200\":\n          description: Ok.\n          content:\n" +
		"            application/json:\n              schema:\n                $ref: \"#/components/schemas/" + op + "Out\"\n" +
		"        default:\n          $ref: \"#/components/responses/Problem\"\n"
	schema := "    " + op + "Out:\n      type: object\n      properties:\n        ok:\n          type: boolean\n"
	edited := strings.Replace(body, "\ncomponents:\n", "\n"+route+"components:\n", 1)
	edited = strings.TrimRight(edited, "\n") + "\n" + schema
	if err := os.WriteFile(file, []byte(edited), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r mergeRepo) mergeTree(left, right string) (string, error) {
	cmd := exec.CommandContext(r.t.Context(), "git")
	cmd.Args = append(cmd.Args, "merge-tree", "--write-tree", left, right)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func newMergeRepo(t *testing.T) mergeRepo {
	t.Helper()
	r := mergeRepo{t: t, dir: t.TempDir()}
	if err := os.CopyFS(filepath.Join(r.dir, mergeSpecDir), os.DirFS("../../api/spec")); err != nil {
		t.Fatal(err)
	}
	attrs, err := os.ReadFile(filepath.Join(mergeRootDir, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(".gitattributes", attrs, 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, r.dir, "init", "-q", "-b", "trunk")
	if err := os.MkdirAll(filepath.Join(r.dir, mergeAPIDir), 0o750); err != nil {
		t.Fatal(err)
	}
	r.generate(errs.All())
	git(t, r.dir, "add", ".")
	git(t, r.dir, "commit", "-q", "-m", "trunk")
	return r
}

func TestGeneratedAPIFiles_routesAddedToDifferentModulesMergeAsText(t *testing.T) {
	t.Parallel()
	r := newMergeRepo(t)

	git(t, r.dir, "switch", "-q", "-c", "ranking-route", "trunk")
	r.addRoute("ranking", "probeRanking")
	r.generate(codesWith(longProbeError))
	git(t, r.dir, "commit", "-q", "-am", "ranking route")

	git(t, r.dir, "switch", "-q", "-c", "referrals-route", "trunk")
	r.addRoute("referrals", "probeReferrals")
	r.generate(codesWith(referralProbeError))
	git(t, r.dir, "commit", "-q", "-am", "referrals route")

	if out, err := r.mergeTree("ranking-route", "referrals-route"); err != nil || strings.Contains(out, "binary") {
		t.Fatalf("git merge-tree --write-tree conflicted: %v\n%s", err, out)
	}
	git(t, r.dir, "merge", "-q", "--no-edit", "ranking-route")

	want := mergeRepo{t: t, dir: t.TempDir()}
	mergedSpecs := os.DirFS(filepath.Join(r.dir, mergeSpecDir))
	if err := os.CopyFS(filepath.Join(want.dir, mergeSpecDir), mergedSpecs); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(want.dir, mergeAPIDir), 0o750); err != nil {
		t.Fatal(err)
	}
	want.generate(codesWith(longProbeError, referralProbeError))
	names := filesUnder(t, filepath.Join(want.dir, mergeAPIDir))
	files := make([]string, 0, len(names)+1)
	files = append(files, mergeBundle)
	for _, name := range names {
		files = append(files, filepath.Join(mergeAPIDir, name))
	}
	for _, rel := range files {
		if readText(t, filepath.Join(r.dir, rel)) != readText(t, filepath.Join(want.dir, rel)) {
			t.Errorf("%s differs between the merged tree and a regeneration of the merged specs", rel)
		}
	}
}

func TestGeneratedAPIFiles_aLongerErrorCodeAddsOnlyItsOwnLinesToTheBaseEnum(t *testing.T) {
	t.Parallel()
	r := newMergeRepo(t)
	before := readText(t, filepath.Join(r.dir, mergeBaseGo))
	r.generate(codesWith(longProbeError))
	after := readText(t, filepath.Join(r.dir, mergeBaseGo))
	var added []string
	for _, line := range strings.Split(after, "\n") {
		if !strings.Contains(before, line+"\n") {
			added = append(added, line)
		}
	}
	if len(added) != 2 {
		t.Fatalf(
			"a new error code changed %d lines of %s, want its const and its case:\n%s",
			len(added),
			mergeBaseGo,
			strings.Join(added, "\n"),
		)
	}
}
