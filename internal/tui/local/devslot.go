package local

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// DefaultSlotName is the implicit slot used when no keyword is given. It
// maps to the historical default ports and data directory so that
// `mise run desktop:dev` (without a keyword) behaves exactly as before.
const DefaultSlotName = "default"

const (
	baseServerPort       = 18731
	baseWebPort          = 8082
	devSlotsRegistryFile = "dev-slots.json"
)

// reservedHostPorts are fixed ports the desktop/local runtime binds outside
// the slot system. The allocator must never hand these to a slot's server,
// otherwise the slot would clash with that service at runtime.
//
//	18732 — ACP tools proxy (internal/workspace/bridge.ACPToolsProxyAddr)
var reservedHostPorts = map[int]struct{}{
	18732: {},
}

// devSlotNamePattern restricts slot keywords to filesystem- and shell-safe
// identifiers. The desktop dev launcher `eval`s the `slot env` output, so this
// pattern is also a security invariant: it guarantees the emitted MEMOH_SLOT
// value carries no whitespace, quotes, or shell metacharacters. Do not relax
// it without re-checking the eval call site in mise.toml.
var devSlotNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,47}$`)

// devSlotPorts is the persisted unit in dev-slots.json.
type devSlotPorts struct {
	ServerPort int `json:"server_port"`
	WebPort    int `json:"web_port"`
}

// DevSlot is the resolved view of a dev slot returned to callers.
type DevSlot struct {
	Name       string `json:"name"`
	ServerPort int    `json:"server_port"`
	WebPort    int    `json:"web_port"`
	DataDir    string `json:"data_dir"`
}

// ValidateDevSlotName reports whether name is an acceptable slot keyword.
func ValidateDevSlotName(name string) error {
	if !devSlotNamePattern.MatchString(name) {
		return fmt.Errorf("invalid slot name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,47}", name)
	}
	return nil
}

// DevSlotsRegistryPath returns the path to the per-user dev slot
// registry (userData/dev-slots.json).
func DevSlotsRegistryPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, devSlotsRegistryFile), nil
}

// DevSlotDataDir returns the per-slot data directory under the desktop
// dev server work dir (userData/local-server). The default slot keeps
// the historical data/local layout; named slots live under
// data/instances/<name>.
func DevSlotDataDir(name string) (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	workDir := filepath.Join(dir, "local-server")
	if name == DefaultSlotName || name == "" {
		return filepath.Join(workDir, "data", "local"), nil
	}
	return filepath.Join(workDir, "data", "instances", name), nil
}

func loadDevSlotRegistry(path string) (map[string]devSlotPorts, error) {
	reg := map[string]devSlotPorts{}
	raw, err := os.ReadFile(path) //nolint:gosec // path derived from UserDataDir
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read dev-slots registry: %w", err)
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("parse dev-slots registry: %w", err)
	}
	return reg, nil
}

func saveDevSlotRegistry(path string, reg map[string]devSlotPorts) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create userData dir: %w", err)
	}
	encoded, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dev-slots registry: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write dev-slots registry: %w", err)
	}
	return os.Rename(tmp, path)
}

// portAvailable reports whether a TCP port can currently be bound on
// loopback. Used to skip ports occupied by foreign processes when
// allocating a new slot.
func portAvailable(port int) bool {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func nextFreePort(start int, used map[int]struct{}) (int, error) {
	for port := start; port <= 65535; port++ {
		if _, taken := used[port]; taken {
			continue
		}
		if portAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port available at or above %d", start)
}

// withRegistryLock serializes the registry read-modify-write across processes
// (parallel worktrees can call ResolveDevSlot at the same time). The atomic
// rename in saveDevSlotRegistry only prevents a torn file, not a lost update,
// so a simple O_EXCL lock file guards the whole load→allocate→save sequence.
func withRegistryLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return fmt.Errorf("create userData dir: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // path derived from UserDataDir
		if err == nil {
			_ = f.Close()
			break
		}
		if !os.IsExist(err) {
			return fmt.Errorf("acquire dev-slots lock: %w", err)
		}
		// Reap a lock left behind by a crashed process.
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for dev-slots lock (%s); remove it if it is stale", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer func() { _ = os.Remove(lockPath) }()
	return fn()
}

// ResolveDevSlot returns the ports and data directory for the named
// slot, allocating and persisting a fresh, free port pair on first use.
// The default slot always maps to the reserved {18731, 8082} pair and
// is never written to the registry.
func ResolveDevSlot(name string) (DevSlot, error) {
	if name == "" {
		name = DefaultSlotName
	}
	if err := ValidateDevSlotName(name); err != nil {
		return DevSlot{}, err
	}
	dataDir, err := DevSlotDataDir(name)
	if err != nil {
		return DevSlot{}, err
	}
	if name == DefaultSlotName {
		return DevSlot{Name: name, ServerPort: baseServerPort, WebPort: baseWebPort, DataDir: dataDir}, nil
	}

	path, err := DevSlotsRegistryPath()
	if err != nil {
		return DevSlot{}, err
	}

	var resolved DevSlot
	lockErr := withRegistryLock(path, func() error {
		reg, err := loadDevSlotRegistry(path)
		if err != nil {
			return err
		}
		if existing, ok := reg[name]; ok {
			resolved = DevSlot{Name: name, ServerPort: existing.ServerPort, WebPort: existing.WebPort, DataDir: dataDir}
			return nil
		}

		usedServer := map[int]struct{}{baseServerPort: {}}
		for p := range reservedHostPorts {
			usedServer[p] = struct{}{}
		}
		usedWeb := map[int]struct{}{baseWebPort: {}}
		for _, p := range reg {
			usedServer[p.ServerPort] = struct{}{}
			usedWeb[p.WebPort] = struct{}{}
		}
		serverPort, err := nextFreePort(baseServerPort+1, usedServer)
		if err != nil {
			return err
		}
		webPort, err := nextFreePort(baseWebPort+1, usedWeb)
		if err != nil {
			return err
		}

		reg[name] = devSlotPorts{ServerPort: serverPort, WebPort: webPort}
		if err := saveDevSlotRegistry(path, reg); err != nil {
			return err
		}
		resolved = DevSlot{Name: name, ServerPort: serverPort, WebPort: webPort, DataDir: dataDir}
		return nil
	})
	if lockErr != nil {
		return DevSlot{}, lockErr
	}
	return resolved, nil
}

// ListDevSlots returns all known dev slots, always including the
// reserved default slot.
func ListDevSlots() ([]DevSlot, error) {
	path, err := DevSlotsRegistryPath()
	if err != nil {
		return nil, err
	}
	reg, err := loadDevSlotRegistry(path)
	if err != nil {
		return nil, err
	}
	names := []string{DefaultSlotName}
	for n := range reg {
		if n != DefaultSlotName {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	slots := make([]DevSlot, 0, len(names))
	for _, n := range names {
		dataDir, dirErr := DevSlotDataDir(n)
		if dirErr != nil {
			return nil, dirErr
		}
		if n == DefaultSlotName {
			slots = append(slots, DevSlot{Name: n, ServerPort: baseServerPort, WebPort: baseWebPort, DataDir: dataDir})
			continue
		}
		p := reg[n]
		slots = append(slots, DevSlot{Name: n, ServerPort: p.ServerPort, WebPort: p.WebPort, DataDir: dataDir})
	}
	return slots, nil
}
