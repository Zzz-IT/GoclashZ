//go:build windows

package clash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goclashz/core/utils"
)

const MainConfigID = "config.yaml"

func NormalizeProfileID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || id == MainConfigID {
		return MainConfigID, nil
	}

	safeID, err := utils.SanitizeFilename(id)
	if err != nil {
		return "", err
	}

	if safeID != id {
		return "", fmt.Errorf("非法配置 ID: %q", id)
	}

	return safeID, nil
}

func ProfilePathByID(id string) (string, string, error) {
	normalizedID, err := NormalizeProfileID(id)
	if err != nil {
		return "", "", err
	}

	if normalizedID == MainConfigID {
		return normalizedID, GetConfigPath(), nil
	}

	baseDir := utils.GetSubscriptionsDir()
	target := filepath.Join(baseDir, normalizedID+".yaml")

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}

	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("配置路径逃逸: %s", id)
	}

	return normalizedID, targetAbs, nil
}

func ProfilePathByIDOrMain(id string) (string, string, error) {
	normalizedID, path, err := ProfilePathByID(id)
	if err != nil {
		return "", "", err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) && normalizedID != MainConfigID {
		return MainConfigID, GetConfigPath(), nil
	}

	return normalizedID, path, nil
}

func ProfileExists(id string) bool {
	normalizedID, err := NormalizeProfileID(id)
	if err != nil {
		return false
	}

	if normalizedID == MainConfigID {
		return true
	}

	_, ok := FindSubIndexByID(normalizedID)
	return ok
}
