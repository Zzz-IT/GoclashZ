//go:build windows

package main

import "goclashz/core/appcore"

type OutboundIPResult = appcore.OutboundIPResult

func (a *App) GetOutboundIP(force bool) (OutboundIPResult, error) {
	return a.core.GetOutboundIP(force)
}

func (a *App) GetOutboundIPForRoute(force bool, expectedRoute string) (OutboundIPResult, error) {
	return a.core.GetOutboundIPForRoute(force, expectedRoute)
}
