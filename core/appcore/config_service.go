package appcore

import (
	"context"
	"goclashz/core/clash"
)

func (c *Controller) SaveTunConfig(ctx context.Context, cfg *clash.TunConfig) error {
	if err := clash.UpdateTunConfig(cfg); err != nil {
		return err
	}
	if c.GetAppState().IsRunning {
		return c.RestartCore(ctx)
	}
	return nil
}

func (c *Controller) SaveDNSConfig(ctx context.Context, cfg *clash.DNSConfig) error {
	if err := clash.UpdateDNSConfig(cfg); err != nil {
		return err
	}
	if c.GetAppState().IsRunning {
		return c.RestartCore(ctx)
	}
	return nil
}

func (c *Controller) SaveNetworkConfig(ctx context.Context, cfg *clash.NetworkConfig) error {
	if err := clash.UpdateNetworkConfig(cfg); err != nil {
		return err
	}
	if c.GetAppState().IsRunning {
		return c.RestartCore(ctx)
	}
	return nil
}
