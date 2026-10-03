package flows_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func rowWithID(id string) string {
	return fundRowWith(func(c []string) { c[0] = id })
}

func flowRepo(files map[string]string) fstest.MapFS {
	repo := fstest.MapFS{}
	for name, body := range files {
		repo[flows.Dir+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return repo
}

func TestReadAll_returnsOneFlowPerFileInIDOrder(t *testing.T) {
	t.Parallel()
	repo := flowRepo(map[string]string{})
	for _, id := range []string{"10", "7a", "07", "1"} {
		repo[flows.Dir+"/"+id+".tsv"] = &fstest.MapFile{Data: []byte(tsv(rowWithID(id)))}
	}
	repo[flows.Dir+"/README.md"] = &fstest.MapFile{Data: []byte("not a flow\n")}
	got, problems, err := flows.ReadAll(repo)
	if err != nil || len(problems) != 0 {
		t.Fatalf("ReadAll = %v, %v", lines(problems), err)
	}
	ids, files := make([]string, 0, len(got)), make([]string, 0, len(got))
	for _, f := range got {
		ids, files = append(ids, f.ID), append(files, f.File)
	}
	if want := []string{"1", "07", "7a", "10"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %q, want %q", ids, want)
	}
	if files[0] != flows.Dir+"/1.tsv" || got[0].Line != 2 {
		t.Fatalf("first flow at %s:%d, want %s/1.tsv:2", files[0], got[0].Line, flows.Dir)
	}
}

func TestReadAll_eachFileProblemNamesTheFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, body, want string
	}{
		{"two rows", "05.tsv", tsv(rowWithID("05"), rowWithID("05")), "packages/flows/backend/05.tsv: has 2 rows, want 1"},
		{"no row", "05.tsv", tsv(), "packages/flows/backend/05.tsv: has 0 rows, want 1"},
		{
			"id differs from the file name", "07.tsv", tsv(rowWithID("08")),
			"packages/flows/backend/07.tsv:2: id 08 differs from the file name; rename the file to 08.tsv or set the id to 07",
		},
		{
			"wrong header", "07.tsv", "id\tflow\n" + fundRow + "\n",
			`packages/flows/backend/07.tsv:1: header must be "` + strings.ReplaceAll(flows.Header, "\t", `\t`) + `"`,
		},
		{"malformed row", "07.tsv", tsv("07\tFund cabal"), "packages/flows/backend/07.tsv:2: has 2 columns, want 10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := flowRepo(map[string]string{tc.file: tc.body, "01.tsv": tsv(rowWithID("01"))})
			got, problems, err := flows.ReadAll(repo)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(lines(problems), []string{tc.want}) {
				t.Fatalf("problems = %q, want %q", lines(problems), tc.want)
			}
			if len(got) != 1 || got[0].ID != "01" {
				t.Fatalf("flows = %v, want only the valid 01", got)
			}
		})
	}
}

func TestReadAll_failsWithNoFlowFiles(t *testing.T) {
	t.Parallel()
	if _, _, err := flows.ReadAll(fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), flows.Dir) {
		t.Fatalf("err = %v, want the missing %s named", err, flows.Dir)
	}
}

func TestReadAll_failsNamingAFileItCannotRead(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{flows.Dir + "/07.tsv/x": {Data: []byte("x\n")}}
	if _, _, err := flows.ReadAll(repo); err == nil || !strings.Contains(err.Error(), "read "+flows.Dir+"/07.tsv") {
		t.Fatalf("err = %v, want the unreadable 07.tsv named", err)
	}
}
