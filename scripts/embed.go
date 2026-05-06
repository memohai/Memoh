package scripts

import _ "embed"

// DesktopInstall is the fallback desktop install script bundled into the binary.
//
//go:embed desktop-install.sh
var DesktopInstall string
