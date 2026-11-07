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
	Root    string // root | sudo | sudo-password | none| sudo | none
	Home    string
}

func (i Info) String() string {
	root := i.Root
	if root == "sudo-password" {
		root = "sudo (needs password)"
	}
	return fmt.Sprintf("%s %s · %s · root: %s", i.OS, i.Version, i.Arch, root)
}

// Remote drives a server through the system ssh binary and one ControlMaster
// connection, so authentication happens once and every step reuses it.
type Remote struct {
	Host      string
	ExtraArgs []string // e.g. -p 2222 -i key -o ...
	ctlDir    string
	ctlPath   string
	Info      Info

	// SudoPassword is held in memory only. It is sent as the first stdin line of
	// steps that need root, never on a command line, in a log, or on disk.
	SudoPassword string
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
//
// If password is non-empty it is sent as the first line of stdin; the wrapper
// reads it into SSS_SUDO_PW (an env var of the step's processes only, so it
// is not visible in `ps` output, a log or a file).
func (r *Remote) runScript(ctx context.Context, script, password string, timeout time.Duration, out io.Writer) (int, error) {
	readPw := ""
	stdin := script
	if password != "" {
		readPw = `IFS= read -r SSS_SUDO_PW; export SSS_SUDO_PW; `
		stdin = password + "\n" + script
	}
	remote := fmt.Sprintf(`bash -c '%sf=$(mktemp /tmp/sss-step.XXXXXX); cat > "$f"; timeout -k 5 %d bash "$f" < /dev/null; rc=$?; rm -f "$f"; exit $rc'`,
		readPw, int(timeout.Seconds()))
	cmd := exec.CommandContext(ctx, "ssh", "-S", r.ctlPath, "-T", r.Host, remote)
	cmd.Stdin = strings.NewReader(stdin)
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
if command -v sudo >/dev/null 2>&1; then
  out=$(sudo -n true 2>&1 </dev/null) && echo sudo=1 || {
    case "$out" in *password*) echo sudo=password ;; esac
  }
fi
`

func (r *Remote) Preflight(ctx context.Context) error {
	var buf bytes.Buffer
	code, err := r.runScript(ctx, preflightScript, "", 60*time.Second, &buf)
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
	case kv["sudo"] == "password":
		// Could also be a user who is not in sudoers at all; VerifySudo tells.
		info.Root = "sudo-password"
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
	// Only steps that may use root ever receive the password.
	pw := ""
	if r.Info.Root == "sudo-password" && (s.NeedsRoot || s.RootOptional) {
		pw = r.SudoPassword
	}
	return r.runScript(ctx, script, pw, s.Timeout, out)
}

// VerifySudo checks a sudo password using the same askpass path as as_root,
// after invalidating any cached sudo timestamp so a wrong password really fails.
func (r *Remote) VerifySudo(ctx context.Context, password string) bool {
	script := `
d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
cat > "$d/askpass" <<'EOF'
#!/bin/sh
printf '%s\n' "$SSS_SUDO_PW"
EOF
chmod 700 "$d/askpass"
sudo -k
SUDO_ASKPASS="$d/askpass" sudo -A true
`
	code, err := r.runScript(ctx, script, password, 60*time.Second, io.Discard)
	return err == nil && code == 0
}
