//go:build windows

package appcore

import (
	"context"
	"goclashz/core/clash"
)

func (c *Controller) SaveTunConfig(ctx context.Context, cfg *clash.TunConfig) error {
	if err := clash.UpdateTunConfig(cfg); err != nil {
		return err
	}
	if clash.IsRunning() {
		return c.RestartCore(ctx)
	}
	return nil
}

func (c *Controller) SaveDNSConfig(ctx context.Context, cfg *clash.DNSConfig) error {
	if err := clash.UpdateDNSConfig(cfg); err != nil {
		return err
	}
	if clash.IsRunning() {
		return c.RestartCore(ctx)
	}
	return nil
}

func (c *Controller) SaveNetworkConfig(ctx context.Context, cfg *clash.NetworkConfig) error {
	if err := clash.UpdateNetworkConfig(cfg); err != nil {
		return err
	}
	if clash.IsRunning() {
		return c.RestartCore(ctx)
	}
	return nil
}
func (c *Controller) ResetTunConfig(ctx context.Context) error {
	defaultCfg := clash.GetDefaultTunConfig()
	return c.SaveTunConfig(ctx, &defaultCfg)
}

func (c *Controller) ResetDNSConfig(ctx context.Context) error {
	defaultCfg := clash.GetDefaultDNSConfig()
	return c.SaveDNSConfig(ctx, &defaultCfg)
}

func (c *Controller) ResetNetworkConfig(ctx context.Context) error {
	defaultCfg := clash.GetDefaultNetworkConfig()
	return c.SaveNetworkConfig(ctx, &defaultCfg)
}
