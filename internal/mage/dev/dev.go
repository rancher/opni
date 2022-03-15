package dev

import (
	"os"
	"os/exec"
	"strings"

	"github.com/kralicky/spellbook/upgrade"
)

func Upgrade() error {
	return upgrade.Upgrade()
}

func TiltLogs() error {
	cmd := exec.Command("/bin/bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = strings.NewReader(`
#!/bin/bash
tilt logs -f | awk 'tolower($0) !~ /health|push|ready|memberlist_logger|cleanup/ {print}'
`)
	return cmd.Run()
}
