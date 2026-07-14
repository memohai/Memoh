package scripts

import _ "embed"

// DesktopInstall is the fallback desktop install script bundled into the binary.
//
//go:embed desktop-install.sh
var DesktopInstall string

// DesktopStyle is the fallback desktop styling script bundled into the binary.
//
//go:embed desktop-style.sh
var DesktopStyle string

// DisplayApplyStyle applies the desktop theme and records its version.
//
//go:embed display-apply-style.sh
var DisplayApplyStyle string

// DisplayPrepare starts the VNC server, desktop session, and headed browser.
//
//go:embed display-prepare.sh
var DisplayPrepare string
