package identity

import (
	"fmt"

	ctr "github.com/memohai/memoh/internal/containerd"
)

// ValidateChannelIdentityID enforces a conservative ID charset for isolation.
func ValidateChannelIdentityID(channelIdentityID string) error {
	if channelIdentityID == "" {
		return fmt.Errorf("%w: channel identity id required", ctr.ErrInvalidArgument)
	}
	for _, r := range channelIdentityID {
		if r != '-' && r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return fmt.Errorf("%w: invalid channel identity id", ctr.ErrInvalidArgument)
		}
	}
	return nil
}
