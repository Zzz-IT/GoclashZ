//go:build windows

package main

import "goclashz/core/appcore"

type OutboundIPResult = appcore.OutboundIPResult

func (a *App) GetOutboundIP(force bool) (OutboundIPResult, error) {
	return a.core.GetOutboundIP(force)
}
