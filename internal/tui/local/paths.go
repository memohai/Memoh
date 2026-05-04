// Package local resolves the on-disk layout shared between the Memoh
// desktop Electron shell (apps/desktop) and the bundled CLI binary
// (cmd/memoh). The two cooperate purely through files under the same
// userData directory: config.toml, local-server.pid.json, qdrant/, etc.
//
// Path rules mirror Electron's app.getPath('userData') with productName
// pinned to "Memoh" (see apps/desktop/package.json and the call to
// app.setName('Memoh') in apps/desktop/src/main/index.ts).
package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const productName = "Memoh"

// LocalServerPort is the fixed port the desktop-managed server binds to.
// Mirrors LOCAL_SERVER_PORT in apps/desktop/src/main/local-server.ts.
const LocalServerPort = 18731

// LocalServerBaseURL is the canonical http endpoint of the
// desktop-managed server. CLI clients address it directly.
const LocalServerBaseURL = "http://127.0.0.1:18731"

// UserDataDir returns the cross-platform Electron-equivalent userData
// directory. The directory is not created here; callers should expect
// it to be missing on a fresh machine.
func UserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", productName), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, productName), nil
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, productName), nil
	}
}

// MustUserDataDir is a convenience wrapper that panics on error. Used
// only in CLI code paths where the userData directory not being
// resolvable is a fatal precondition.
func MustUserDataDir() string {
	dir, err := UserDataDir()
	if err != nil {
		panic(err)
	}
	return dir
}

// ConfigPath returns the path where a packaged Memoh.app writes
// config.toml. Use ResolveConfigPath when you need to read it — that
// helper also recognizes the dev-mode location.
func ConfigPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// devConfigPath mirrors `desktopServerWorkDir()` in
// apps/desktop/src/main/paths.ts: in dev (electron-vite) the desktop
// renders config.toml into userData/local-server/config.toml so it
// can sit next to the freshly-built dev server binary.
func devConfigPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-server", "config.toml"), nil
}

// ResolveConfigPath returns whichever config.toml actually exists on
// disk — the packaged location takes precedence, falling back to the
// dev location. Returns an error with both candidates listed when
// neither file is present so users know to launch the desktop once.
func ResolveConfigPath() (string, error) {
	packaged, err := ConfigPath()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(packaged); statErr == nil {
		return packaged, nil
	}
	dev, err := devConfigPath()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(dev); statErr == nil {
		return dev, nil
	}
	return "", fmt.Errorf("config.toml not found in either %q or %q; open Memoh.app once to initialize", packaged, dev)
}

// PidPath returns the path to the desktop-managed server's pid file.
func PidPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-server.pid.json"), nil
}

// LogPath returns the path to the desktop-managed server's log file.
func LogPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-server.log"), nil
}

// TokenCachePath returns the path used by SaveCachedToken/LoadCachedToken.
func TokenCachePath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli-token.json"), nil
}

// PrefsPath returns the path used to persist CLI/desktop preferences
// (e.g. dontAskAgain for the install-CLI prompt).
func PrefsPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli-prefs.json"), nil
}

// QdrantPidPath returns the path to the embedded qdrant pid file.
func QdrantPidPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qdrant", "qdrant.pid.json"), nil
}

// QdrantPortsPath returns the path to the json file recording the
// dynamically-assigned qdrant ports.
func QdrantPortsPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qdrant", "ports.json"), nil
}

// BundledServerBinary returns the absolute path to the memoh-server
// binary shipped alongside the CLI inside the desktop app bundle.
//
// Layout inside a packaged Memoh.app on macOS:
//
//	Memoh.app/Contents/Resources/cli/memoh        <- the CLI itself
//	Memoh.app/Contents/Resources/server/memoh-server
//
// Layout in dev (running `go run ./cmd/memoh`): no bundled binary; the
// returned error gives callers a chance to fail gracefully with a
// human-actionable message.
func BundledServerBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve own executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}
	cliDir := filepath.Dir(exe)
	resourcesDir := filepath.Dir(cliDir)
	candidate := filepath.Join(resourcesDir, "server", serverBinaryName())
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", errors.New("bundled memoh-server binary not found; CLI must run from a packaged Memoh app")
}

func serverBinaryName() string {
	if runtime.GOOS == "windows" {
		return "memoh-server.exe"
	}
	return "memoh-server"
}
