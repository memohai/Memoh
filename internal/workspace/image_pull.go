package workspace

import (
	"context"
	"errors"
	"log/slog"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
)

type ImagePrepareMode string

const (
	ImagePreparePulled    ImagePrepareMode = "pulled"
	ImagePrepareSkipped   ImagePrepareMode = "skipped"
	ImagePrepareDelegated ImagePrepareMode = "delegated"
)

type ImagePrepareResult struct {
	Mode    ImagePrepareMode
	Image   ctr.ImageInfo
	Message string
}

func (m *Manager) PrepareImageForCreate(ctx context.Context, image string, opts *ctr.PullImageOptions) (ImagePrepareResult, error) {
	policy := m.cfg.EffectiveImagePullPolicy()
	if policy == config.ImagePullPolicyNever {
		return ImagePrepareResult{Mode: ImagePrepareSkipped, Message: "image pull disabled by policy"}, nil
	}

	imageService, ok := m.service.(ctr.ImageService)
	if !ok {
		return ImagePrepareResult{Mode: ImagePrepareDelegated, Message: "container backend handles image pulling"}, nil
	}

	if policy == config.ImagePullPolicyIfNotPresent {
		info, err := imageService.GetImage(ctx, image)
		if err == nil {
			return ImagePrepareResult{Mode: ImagePrepareSkipped, Image: info, Message: "image already present"}, nil
		}
		if errors.Is(err, ctr.ErrNotSupported) {
			return ImagePrepareResult{Mode: ImagePrepareDelegated, Message: "container backend handles image pulling"}, nil
		}
		if !ctr.IsNotFound(err) {
			m.logger.Info("image lookup failed, attempting pull",
				slog.String("image", image),
				slog.Any("error", err))
		}
	}

	info, err := imageService.PullImage(ctx, image, opts)
	if err != nil {
		if errors.Is(err, ctr.ErrNotSupported) {
			return ImagePrepareResult{Mode: ImagePrepareDelegated, Message: "container backend handles image pulling"}, nil
		}
		return ImagePrepareResult{}, err
	}
	return ImagePrepareResult{Mode: ImagePreparePulled, Image: info, Message: "image pulled"}, nil
}
