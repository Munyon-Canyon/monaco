package scripts_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPreReport_printsLocalGatesFromTheCIFilters(t *testing.T) {
	script := repoRoot(t) + "/scripts/pre-report.sh"
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{
			"openapi runs mobile-core and the app build",
			[]string{"apps/backend/api/openapi.yaml"},
			"just test backend\ncd packages/mobile-core && swift test\njust build mobile\n",
		},
		{
			"xcode-version runs the app build",
			[]string{".xcode-version"},
			"cd packages/mobile-core && swift test\njust build mobile\n",
		},
		{
			"error codes stay on the backend gate",
			[]string{"apps/backend/internal/errs/codes.go"},
			"just test backend\n",
		},
		{
			"backend only",
			[]string{"apps/backend/internal/platform/db/db.go"},
			"just test backend\n",
		},
		{
			"mobile-core only",
			[]string{"packages/mobile-core/Sources/Foo.swift"},
			"cd packages/mobile-core && swift test\njust build mobile\n",
		},
		{
			"ios only",
			[]string{"apps/mobile/App.swift"},
			"just build mobile\n",
		},
		{
			"ci only",
			[]string{".github/workflows/ci.yml"},
			"docker run --rm -v \"$PWD:/repo\" -w /repo rhysd/actionlint:1.7.12\n",
		},
		{
			"scripts go",
			[]string{"scripts/foo.go"},
			"cd scripts && go test ./...\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", append([]string{script}, tc.files...)...)
			cmd.Env = append(os.Environ(), "PYENV_VERSION=system")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("pre-report.sh: %v\n%s", err, out)
			}
			if string(out) != tc.want {
				t.Fatalf("got %q\nwant %q", out, tc.want)
			}
			if strings.Contains(string(out), "count=20") {
				t.Fatal("pre-report printed the flake rerun")
			}
		})
	}
}
