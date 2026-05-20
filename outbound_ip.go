//go:build windows

package main

import "goclashz/core/appcore"

type OutboundIPResult = appcore.OutboundIPResult

func (a *App) GetOutboundIP() (OutboundIPResult, error) {
	return a.core.GetOutboundIP()
}
