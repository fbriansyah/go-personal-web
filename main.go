package main

import (
	"fmt"
	"os"

	"github.com/fbriansyah/go-personal-web/internal/cli"
)

// version is stamped in at build time:
//
//	go build -ldflags "-X main.version=v1.2.0"
var version = "dev"

func main() {
	if err := cli.NewRootCmd(cli.NewBuildInfo(version)).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
