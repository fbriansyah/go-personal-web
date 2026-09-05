package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/fbriansyah/go-personal-web/internal/cli"
)

// shown mirrors the output of `config show`.
type shown struct {
	LogLevel string `yaml:"log_level"`
	Server   struct {
		Port        int    `yaml:"port"`
		ReadTimeout string `yaml:"read_timeout"`
	} `yaml:"server"`
	Database struct {
		URL             string `yaml:"url"`
		Pool            int    `yaml:"pool"`
		ConnMaxLifetime string `yaml:"conn_max_lifetime"`
	} `yaml:"database"`
}

// run builds a fresh command tree, executes it, and returns its stdout.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd := cli.NewRootCmd(cli.BuildInfo{Version: "test"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

// showConfig runs `config show` and parses the effective config it prints.
func showConfig(t *testing.T, args ...string) shown {
	t.Helper()

	out, err := run(t, append([]string{"config", "show"}, args...)...)
	if err != nil {
		t.Fatalf("config show: %v\n%s", err, out)
	}

	var got shown
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parsing output: %v\n%s", err, out)
	}
	return got
}

// writeConfig writes a config file into a temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// isolate points HOME at an empty directory so that a config file in the
// developer's real home cannot influence the test.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestPrecedence(t *testing.T) {
	file := writeConfig(t, "server:\n  port: 3000\n")

	tests := []struct {
		name     string
		env      string
		args     []string
		wantPort int
	}{
		{
			name:     "default when nothing else sets it",
			args:     nil,
			wantPort: 8080,
		},
		{
			name:     "config file beats default",
			args:     []string{"--config", file},
			wantPort: 3000,
		},
		{
			name:     "env beats config file",
			env:      "4000",
			args:     []string{"--config", file},
			wantPort: 4000,
		},
		{
			name:     "flag beats env",
			env:      "4000",
			args:     []string{"--config", file, "--port", "5000"},
			wantPort: 5000,
		},
		{
			name:     "flag beats config file",
			args:     []string{"--config", file, "--port", "5000"},
			wantPort: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			if tt.env != "" {
				t.Setenv("PW_SERVER_PORT", tt.env)
			}

			if got := showConfig(t, tt.args...).Server.Port; got != tt.wantPort {
				t.Errorf("server.port = %d, want %d", got, tt.wantPort)
			}
		})
	}
}

// An unset flag must rank below every other source, otherwise its zero default
// would silently win over the config file.
func TestUnsetFlagDoesNotOverrideConfigFile(t *testing.T) {
	isolate(t)
	file := writeConfig(t, "server:\n  read_timeout: 30s\nlog_level: warn\n")

	got := showConfig(t, "--config", file)
	if got.Server.ReadTimeout != "30s" {
		t.Errorf("server.read_timeout = %q, want %q", got.Server.ReadTimeout, "30s")
	}
	if got.LogLevel != "warn" {
		t.Errorf("log_level = %q, want %q", got.LogLevel, "warn")
	}
}

func TestMissingConfigFile(t *testing.T) {
	t.Run("discovered file may be absent", func(t *testing.T) {
		isolate(t)

		if got := showConfig(t).Server.Port; got != 8080 {
			t.Errorf("server.port = %d, want the default 8080", got)
		}
	})

	t.Run("explicit file must exist", func(t *testing.T) {
		isolate(t)
		missing := filepath.Join(t.TempDir(), "nope.yaml")

		_, err := run(t, "config", "show", "--config", missing)
		if err == nil {
			t.Fatal("expected an error for a missing --config file")
		}
		if !strings.Contains(err.Error(), "nope.yaml") {
			t.Errorf("error does not name the file: %v", err)
		}
	})
}

func TestRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown key",
			body: "server:\n  prot: 8080\n",
			want: "prot",
		},
		{
			name: "port out of range",
			body: "server:\n  port: 99999\n",
			want: "server.port",
		},
		{
			name: "unknown log level",
			body: "log_level: verbose\n",
			want: "log_level",
		},
		{
			name: "non-positive timeout",
			body: "server:\n  read_timeout: 0s\n",
			want: "server.read_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)

			_, err := run(t, "config", "show", "--config", writeConfig(t, tt.body))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestConfigShowNamesTheFile(t *testing.T) {
	isolate(t)
	file := writeConfig(t, "log_level: debug\n")

	out, err := run(t, "config", "show", "--config", file)
	if err != nil {
		t.Fatalf("config show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "# config file: "+file) {
		t.Errorf("output does not name the config file:\n%s", out)
	}
}

func TestVersion(t *testing.T) {
	out, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "personal-web test") {
		t.Errorf("unexpected version output: %q", out)
	}
}

// Each invocation must get its own state; nothing may leak between trees.
func TestCommandTreeIsIndependent(t *testing.T) {
	isolate(t)

	if got := showConfig(t, "--port", "5000").Server.Port; got != 5000 {
		t.Fatalf("server.port = %d, want 5000", got)
	}
	if got := showConfig(t).Server.Port; got != 8080 {
		t.Errorf("server.port = %d on a fresh tree, want the default 8080", got)
	}
}

func TestUnknownFlagShowsUsage(t *testing.T) {
	isolate(t)

	out, err := run(t, "serve", "--nope")
	if err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("a parse error should still print usage:\n%s", out)
	}
}

// `config show` is an Ops Command, so it has to stay safe to run against a live
// system even now that the Effective Config carries a database password.
func TestConfigShowRedactsTheDatabasePassword(t *testing.T) {
	isolate(t)
	t.Setenv("PW_DATABASE_URL", "postgres://writer:hunter2@db.example.com:5432/pw?sslmode=require")

	out, err := run(t, "config", "show")
	if err != nil {
		t.Fatalf("config show: %v\n%s", err, out)
	}
	if strings.Contains(out, "hunter2") {
		t.Fatalf("config show printed the password:\n%s", out)
	}

	var got shown
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parsing output: %v\n%s", err, out)
	}
	const want = "postgres://writer:xxxxx@db.example.com:5432/pw?sslmode=require"
	if got.Database.URL != want {
		t.Errorf("database.url = %q, want %q", got.Database.URL, want)
	}
	// The rest of the DSN survives, because host and database name are what
	// makes the command worth running when a deployment points somewhere odd.
	if got.Database.Pool != 5 {
		t.Errorf("database.pool = %d, want 5", got.Database.Pool)
	}
}

// The database obeys the same Config Source ranking as everything else, which
// is the whole reason it is not configured by database.yml.
func TestDatabaseURLFollowsPrecedence(t *testing.T) {
	isolate(t)
	path := writeConfig(t, "database:\n  url: postgres://file@filehost:5432/pw\n")

	got := showConfig(t, "--config", path)
	if !strings.Contains(got.Database.URL, "filehost") {
		t.Fatalf("config file did not win over the default: %q", got.Database.URL)
	}

	t.Setenv("PW_DATABASE_URL", "postgres://env@envhost:5432/pw")
	got = showConfig(t, "--config", path)
	if !strings.Contains(got.Database.URL, "envhost") {
		t.Errorf("environment did not win over the config file: %q", got.Database.URL)
	}
}

// An unusable DSN must fail while resolving config, not later at connect time.
func TestRejectsBadDatabaseURL(t *testing.T) {
	isolate(t)
	t.Setenv("PW_DATABASE_URL", "mysql://u:p@host:3306/db")

	out, err := run(t, "config", "show")
	if err == nil {
		t.Fatalf("expected an error, got:\n%s", out)
	}
	if strings.Contains(err.Error(), "p@host") {
		t.Errorf("the error leaked the DSN: %v", err)
	}
}
