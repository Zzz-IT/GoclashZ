package appcore

import (
	"context"
	"goclashz/core/sys"
)

func (c *Controller) InstallTunDriverAsync(ctx context.Context) {
	c.Tasks.Run(ctx, "driver-install", true, func(ctx context.Context) error {
		_, err := sys.InstallWintun(ctx, false)
		return err
	})
}
