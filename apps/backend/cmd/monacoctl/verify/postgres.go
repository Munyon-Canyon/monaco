package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const (
	PostgresImage   = "public.ecr.aws/docker/library/postgres:16-alpine"
	MirrorImage     = "mirror.gcr.io/library/postgres:16-alpine"
	ContainerPrefix = "monaco-verify-"
	ContainerLabel  = "monaco.verify"
	firstRetryWait  = 2 * time.Second
	secondRetryWait = 5 * time.Second
)

var errPull = errors.New("pull postgres image")

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
	return d.ensureImageWith(ctx, sleepCtx)
}

func sleepCtx(ctx context.Context, wait time.Duration) error {
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait to retry the pull: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

type sleepFunc func(context.Context, time.Duration) error

func (d Docker) ensureImageWith(ctx context.Context, sleep sleepFunc) error {
	if _, err := d.run(ctx, "image", "inspect", PostgresImage); err == nil {
		return nil
	}
	var failures []string
	for _, src := range []string{PostgresImage, MirrorImage} {
		err := d.pullWithRetry(ctx, src, sleep)
		if err == nil {
			return d.tagAsPostgresImage(ctx, src)
		}
		if ctx.Err() != nil {
			return err
		}
		failures = append(failures, src+": "+err.Error())
	}
	return fmt.Errorf("%w: %s", errPull, strings.Join(failures, "; "))
}

func (d Docker) pullWithRetry(ctx context.Context, src string, sleep sleepFunc) error {
	_, err := d.run(ctx, "pull", "-q", src)
	for _, wait := range []time.Duration{firstRetryWait, secondRetryWait} {
		if err == nil {
			return nil
		}
		if slept := sleep(ctx, wait); slept != nil {
			return slept
		}
		_, err = d.run(ctx, "pull", "-q", src)
	}
	return err
}

func (d Docker) tagAsPostgresImage(ctx context.Context, src string) error {
	if src == PostgresImage {
		return nil
	}
	_, err := d.run(ctx, "tag", src, PostgresImage)
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
