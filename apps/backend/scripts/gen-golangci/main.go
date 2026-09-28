package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	beginMarker = "# BEGIN GENERATED depguard"
	endMarker   = "# END GENERATED depguard"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := generate(root); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gen-golangci: %v\n", err)
		os.Exit(1)
	}
}

func generate(root string) error {
	modPath, err := modulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	names, err := moduleNames(filepath.Join(root, "internal", "modules"))
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(root, ".golangci.yml")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	out, err := splice(cfg, modPath, names)
	if err != nil {
		return fmt.Errorf("%s: %w", cfgPath, err)
	}
	if bytes.Equal(out, cfg) {
		return nil
	}
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func modulePath(goMod string) (string, error) {
	b, err := os.ReadFile(goMod)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if path, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(path), `"`), nil
		}
	}
	return "", fmt.Errorf("%s has no module line", goMod)
}

func moduleNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func splice(cfg []byte, modPath string, names []string) ([]byte, error) {
	lines := strings.SplitAfter(string(cfg), "\n")
	begin := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == beginMarker })
	end := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == endMarker })
	if begin < 0 || end < begin {
		return nil, fmt.Errorf("want one %q line followed by one %q line", beginMarker, endMarker)
	}
	indent := lines[begin][:len(lines[begin])-len(strings.TrimLeft(lines[begin], " "))]
	var b strings.Builder
	for _, l := range lines[:begin+1] {
		b.WriteString(l)
	}
	for _, name := range names {
		writeRule(&b, indent, modPath+"/internal/modules/", name)
	}
	for _, l := range lines[end:] {
		b.WriteString(l)
	}
	return []byte(b.String()), nil
}

func writeRule(b *strings.Builder, indent, modulesPkg, name string) {
	own := modulesPkg + name
	fmt.Fprintf(b, "%smodule-%s:\n", indent, name)
	fmt.Fprintf(b, "%s  list-mode: lax\n", indent)
	fmt.Fprintf(b, "%s  files: [\"**/internal/modules/%s/**\"]\n", indent, name)
	fmt.Fprintf(b, "%s  allow: [\"%s$\", \"%s/\"]\n", indent, own, own)
	fmt.Fprintf(b, "%s  deny:\n", indent)
	fmt.Fprintf(b, "%s    - pkg: \"%s\"\n", indent, modulesPkg)
	fmt.Fprintf(b, "%s      desc: \"modules never import each other; send an event or use a query port\"\n", indent)
}
