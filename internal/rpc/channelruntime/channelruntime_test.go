package channelruntime

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/memohai/memoh/internal/channel"
)

func TestSafeChannelErrorUsesStableReasonWithoutCause(t *testing.T) {
	err := safeChannelError(errors.Join(channel.ErrEnableChannelFailed, errors.New("private adapter token")))
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("code = %v", got)
	}
	if got := status.Convert(err).Message(); got != reasonEnableFailed {
		t.Fatalf("message = %q", got)
	}
}

func TestSafeChannelErrorLeavesUnknownCauseForRuntimeSanitization(t *testing.T) {
	cause := errors.New("private database detail")
	if got := safeChannelError(cause); !errors.Is(got, cause) {
		t.Fatalf("error = %v", got)
	}
}
