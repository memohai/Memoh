package containerd

import (
	"os"
	"path/filepath"
	"strings"
)

// TimezoneSpec returns environment variables that propagate the host timezone
// into the container without relying on file bind mounts like /etc/localtime.
// File mounts can fail for workspace containers when the target path is absent
// in the unpacked rootfs, while TZ is sufficient for Go, Node, and most tools.
func TimezoneSpec() ([]MountSpec, []string) {
	var env []string
	if tz := detectTimezone(); tz != "" {
		env = append(env, "TZ="+tz)
	}
	return nil, env
}

func detectTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		return tz
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := strings.TrimSpace(string(data)); tz != "" {
			return tz
		}
	}
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		const zoneinfoPrefix = "/usr/share/zoneinfo/"
		if strings.HasPrefix(target, zoneinfoPrefix) {
			return strings.TrimPrefix(target, zoneinfoPrefix)
		}
	}
	return ""
}
