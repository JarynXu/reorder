package main

import (
	"os"
	"runtime/debug"

	"github.com/JarynXu/reorder/internal/cli"
)

// version is set for release binaries with -ldflags "-X main.version=vX.Y.Z".
var version string

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, buildVersion()))
}

func buildVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}
