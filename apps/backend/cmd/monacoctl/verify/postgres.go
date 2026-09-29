package verify

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

const (
	PostgresImage   = "postgres:16-alpine"
	ContainerPrefix = "monaco-verify-"
	ContainerLabel  = "monaco.verify"
)

type Docker string

func (d Docker) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, string(d), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, bytes.TrimSpace(stderr.Bytes()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (d Docker) ensureImage(ctx context.Context) error {
	if _, err := d.run(ctx, "image", "inspect", PostgresImage); err == nil {
		return nil
	}
	_, err := d.run(ctx, "pull", "-q", PostgresImage)
	return err
}

type postgres struct {
	docker Docker
	name   string
	url    string
}

func startPostgres(ctx context.Context, d Docker, runID string) (*postgres, error) {
	pg := &postgres{docker: d, name: ContainerPrefix + runID}
	_, err := d.run(ctx, "run", "-d", "--rm", "--name", pg.name, "--label", ContainerLabel+"="+runID,
		"-p", "127.0.0.1::5432", "--tmpfs", "/var/lib/postgresql/data",
		"-e", "POSTGRES_USER=monaco", "-e", "POSTGRES_PASSWORD=monaco", "-e", "POSTGRES_DB=monaco",
		PostgresImage, "postgres", "-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off")
	if err != nil {
		return pg, err
	}
	out, err := d.run(ctx, "port", pg.name, "5432/tcp")
	if err != nil {
		return pg, err
	}
	first, _, _ := strings.Cut(out, "\n")
	host, port, err := net.SplitHostPort(first)
	if err != nil {
		return pg, fmt.Errorf("docker port %s printed %q: %w", pg.name, out, err)
	}
	pg.url = "postgres://monaco:monaco@" + net.JoinHostPort(host, port) + "/monaco?sslmode=disable"
	return pg, nil
}

func (p *postgres) remove(ctx context.Context) error {
	_, err := p.docker.run(ctx, "rm", "-f", "-v", p.name)
	if err != nil && strings.Contains(err.Error(), "No such container") {
		return nil
	}
	return err
}
