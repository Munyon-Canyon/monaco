package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var errAdminRoleRequired = errors.New("openapi lint: admin operations require x-admin-role")

func toolOpenapi(env toolEnv) tool {
	return func(args []string, _ io.Writer, stderr io.Writer) int {
		if len(args) != 1 || args[0] != "lint" {
			_, _ = fmt.Fprintln(stderr, "usage: monacoctl openapi lint")
			return 2
		}
		spec, err := os.ReadFile(filepath.Join(env.wd, "api", "openapi.yaml"))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "openapi lint: %v\n", err)
			return 1
		}
		if err := lintAdminRoles(spec); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
}

func lintAdminRoles(spec []byte) error {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return fmt.Errorf("openapi lint: %w", err)
	}
	var missing []string
	for path, item := range doc.Paths.Map() {
		if !strings.HasPrefix(path, "/v1/admin/") {
			continue
		}
		for method, operation := range item.Operations() {
			role, ok := operation.Extensions["x-admin-role"].(string)
			if !ok || (role != "viewer" && role != "moderator" && role != "operator") {
				missing = append(missing, strings.ToUpper(method)+" "+path)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return fmt.Errorf("%w: %s", errAdminRoleRequired, strings.Join(missing, ", "))
}
