//go:build windows

package downloader

import (
	"context"
)

func DownloadAtomic(ctx context.Context, opt Options) error {
	if opt.Mode == "" {
		if opt.Resume {
			opt.Mode = ModeResume
		} else {
			opt.Mode = ModeLargeAsset
		}
	}

	switch opt.Mode {
	case ModeSmallFile:
		return FetchSmallFileAtomic(ctx, opt)
	case ModeResume:
		return DownloadResumeAtomic(ctx, opt)
	case ModeLargeAsset:
		return DownloadLargeAssetAtomic(ctx, opt)
	default:
		return DownloadLargeAssetAtomic(ctx, opt)
	}
}
