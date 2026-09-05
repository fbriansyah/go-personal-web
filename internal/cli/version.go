package cli

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// BuildInfo identifies the running binary.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
	Dirty   bool
}

// NewBuildInfo combines the version stamped in by -ldflags with the VCS
// information the Go toolchain embeds automatically, so that `go run .` and
// `go install` report a usable revision without any build flags.
func NewBuildInfo(version string) BuildInfo {
	bi := BuildInfo{Version: version}
	if bi.Version == "" {
		bi.Version = "dev"
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return bi
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			bi.Commit = s.Value
		case "vcs.time":
			bi.Date = s.Value
		case "vcs.modified":
			bi.Dirty = s.Value == "true"
		}
	}
	return bi
}

// String renders as: personal-web v1.2.0 (a1b2c3d, dirty, 2026-09-05T10:00:00Z)
func (b BuildInfo) String() string {
	var parts []string
	if b.Commit != "" {
		commit := b.Commit
		if len(commit) > 7 {
			commit = commit[:7]
		}
		parts = append(parts, commit)
	}
	if b.Dirty {
		parts = append(parts, "dirty")
	}
	if b.Date != "" {
		parts = append(parts, b.Date)
	}

	s := "personal-web " + b.Version
	if len(parts) > 0 {
		s += " (" + strings.Join(parts, ", ") + ")"
	}
	return s
}

func newVersionCmd(bi BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), bi)
			return err
		},
	}
}
