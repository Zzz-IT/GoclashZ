//go:build windows

package main

import (
	"context"
	"time"

	"goclashz/core/clash"
)

type ConfigTextResult = clash.ConfigTextResult

func (a *App) ReadConfigText(id string) (ConfigTextResult, error) {
	return clash.ReadConfigText(id)
}

func (a *App) SaveConfigText(id string, content string) error {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.core.SaveConfigText(ctx, id, content)
}

func (a *App) ValidateConfigText(content string) error {
	return clash.ValidateConfigText(content)
}

func (a *App) GetConfigFilePath(id string) string {
	return clash.GetConfigFilePath(id)
}

func (a *App) IsConfigEditable(id string) bool {
	return clash.IsConfigEditable(id)
}

func (a *App) GetEditableConfigs() []ConfigTextResult {
	return clash.GetEditableConfigs()
}
