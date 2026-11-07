package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Info is what the preflight learns about the server.
type Info struct {
	OS      string // debian, ubuntu, ...
	Version string
	Arch    string // x86_64 | aarch64
	Root    string // root | sudo | none
	Home    string
}

func (i Info) String() string {
	return fmt.Sprintf("%s %s · %s · root: %s", i.OS, i.Version, i.Arch, i.Root)
}

// Remote drives a server through the system ssh binary and one ControlMaster
// connection, so authentication happens once and every step reuses it.
type Remote struct {
	Host      string
	ExtraArgs []string // e.g. -p 2222 -i key -o ...
	ctlDir    string
	ctlPath   string
	Info      Info
}

func NewRemote(host string, extra []string) (*Remote, error) {
	// Unix socket paths are limited to ~104 bytes on macOS, so keep this short.
	dir, err := os.MkdirTemp("/tmp", "sss-")
	if err != nil {
		return nil, err
	}
	return &Remote{Host: host, ExtraArgs: extra, ctlDir: dir, ctlPath: filepath.Join(dir, "c")}, nil
}

// Connect opens the master connection with the terminal attached, so password
// and host-key prompts work normally. It must run before any TUI starts.
func (r *Remote) Connect() error {
	args := []string{"-M", "-S", r.ctlPath, "-o", "ControlPersist=30m",
		"-o", "ServerAliveInterval=20", "-N", "-f"}
	args = append(args, r.ExtraArgs...)
	args = append(args, r.Host)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh connection to %s failed: %w", r.Host, err)
	}
	return nil
}

func (r *Remote) Close() {
	if r.ctlPath == "" {
		return
	}
	exec.Command("ssh", "-S", r.ctlPath, "-O", "exit", r.Host).Run()
	os.RemoveAll(r.ctlDir)
}

var envPinRe = regexp.MustCompile(`^SSS_[A-Z0-9_]+_VERSION=`)

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// prelude is prepended to every script: facts from the preflight plus any
// SSS_*_VERSION pins from the local environment.
func (r *Remote) prelude() string {
	var b strings.Builder
	fmt.Fprintf(&b, "export SSS_ARCH=%s\nexport SSS_ROOT=%s\n", shq(r.Info.Arch), shq(r.Info.Root))
	var pins []string
	for _, kv := range os.Environ() {
		if envPinRe.MatchString(kv) {
			pins = append(pins, kv)
		}
	}
	sort.Strings(pins)
	for _, kv := range pins {
		k, v, _ := strings.Cut(kv, "=")
		fmt.Fprintf(&b, "export %s=%s\n", k, shq(v))
	}
	return b.String()
}

// runScript pipes script to the server and runs it with `bash`. The command is
// wrapped in `bash -c '...'` using only syntax valid in bash and fish, because
// the remote login shell may well be fish after the first run.
func (r *Remote) runScript(ctx context.Context, script string, timeout time.Duration, out io.Writer) (int, error) {
	remote := fmt.Sprintf(`bash -c 'f=$(mktemp /tmp/sss-step.XXXXXX); cat > "$f"; timeout -k 5 %d bash "$f" < /dev/null; rc=$?; rm -f "$f"; exit $rc'`,
		int(timeout.Seconds()))
	cmd := exec.CommandContext(ctx, "ssh", "-S", r.ctlPath, "-T", r.Host, remote)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

const preflightScript = `
. /etc/os-release 2>/dev/null
echo "os=${ID:-unknown}"
echo "version=${VERSION_ID:-}"
echo "arch=$(uname -m)"
echo "uid=$(id -u)"
echo "home=$HOME"
if sudo -n true 2>/dev/null; then echo sudo=1; fi
`

func (r *Remote) Preflight(ctx context.Context) error {
	var buf bytes.Buffer
	code, err := r.runScript(ctx, preflightScript, 60*time.Second, &buf)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("preflight failed (exit %d): %s", code, strings.TrimSpace(buf.String()))
	}
	kv := map[string]string{}
	for _, l := range strings.Split(buf.String(), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			kv[k] = v
		}
	}
	info := Info{OS: kv["os"], Version: kv["version"], Home: kv["home"]}
	switch kv["arch"] {
	case "x86_64", "amd64":
		info.Arch = "x86_64"
	case "aarch64", "arm64":
		info.Arch = "aarch64"
	default:
		return fmt.Errorf("unsupported architecture %q (x86_64 and arm64 only)", kv["arch"])
	}
	switch {
	case kv["uid"] == "0":
		info.Root = "root"
	case kv["sudo"] == "1":
		info.Root = "sudo"
	default:
		info.Root = "none"
	}
	r.Info = info
	return nil
}

// RunStep runs one step script remotely, streaming combined output to out.
func (r *Remote) RunStep(ctx context.Context, s *Step, common string, out io.Writer) (int, error) {
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return -1, err
	}
	script := r.prelude() + common + "\n" + string(body)
	return r.runScript(ctx, script, s.Timeout, out)
}
