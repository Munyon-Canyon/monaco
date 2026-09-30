package main

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func bindPush(ctx context.Context, d *module.Deps) error {
	if d.Config.APNs.KeyP8 == "" {
		observability.Info(ctx, observability.BootPushDisabled, slog.String("service", "worker"),
			slog.String("reason", "APNS_KEY_P8 is empty"))
		d.APNs = apns.NoopSender{}
		return nil
	}
	client, err := apns.New(d.Config)
	if err != nil {
		return err
	}
	d.APNs = client
	return nil
}
