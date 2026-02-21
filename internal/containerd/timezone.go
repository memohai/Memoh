package containerd

import (
	"os"
)

// TimezoneSpec returns mount specs and environment variables that propagate the host
// timezone into the container via /etc/localtime bind-mount and TZ environment variable.
func TimezoneSpec() ([]MountSpec, []string) {
	var mounts []MountSpec
	var env []string
	if _, err := os.Stat("/etc/localtime"); err == nil {
		mounts = append(mounts, MountSpec{
			Destination: "/etc/localtime",
			Type:        "bind",
			Source:      "/etc/localtime",
			Options:     []string{"rbind", "ro"},
		})
	}
	if tz := os.Getenv("TZ"); tz != "" {
		env = append(env, "TZ="+tz)
	}
	return mounts, env
}
