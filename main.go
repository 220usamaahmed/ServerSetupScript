// sss (ServerSetupScript): set up a fresh server over SSH from a checklist of
// shell-script steps. See README.md.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const usage = `usage: sss [options] [user@]host

Sets up a remote Debian/Ubuntu server from a checklist of steps (steps/*.sh).

options:
  -y, --yes            skip the checklist; run the saved selection (plain output)
      --only a,b       run only these step ids (overrides the saved selection)
      --skip a,b       leave these step ids out
      --steps-dir DIR  steps directory (default: steps/ next to the sss binary,
                       or $SSS_STEPS_DIR)
  -p PORT              ssh port
  -i FILE              ssh identity file
  -o OPTION            extra ssh option (repeatable), e.g. -o ProxyJump=bastion
  -v, --verbose        with --yes/no TTY: also print every output line
      --force          continue on an OS other than Debian/Ubuntu
      --list           list available steps and exit
  -h, --help           show this help

Version pins: SSS_ZELLIJ_VERSION, SSS_HELIX_VERSION, SSS_YAZI_VERSION in the
environment pin that tool instead of installing the latest release.
`

type options struct {
	host      string
	yes       bool
	only      []string
	skip      []string
	stepsDir  string
	ssh       []string
	verbose   bool
	force     bool
	list      bool
	onlyGiven bool
}

func parseArgs(args []string) (*options, error) {
	o := &options{}
	need := func(i int, flag string) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		return args[i+1], nil
	}
	split := func(v string) []string {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			fmt.Print(usage)
			os.Exit(0)
		case "-y", "--yes":
			o.yes = true
		case "-v", "--verbose":
			o.verbose = true
		case "--force":
			o.force = true
		case "--list":
			o.list = true
		case "--only", "--skip", "--steps-dir", "-p", "-i", "-o":
			v, err := need(i, a)
			if err != nil {
				return nil, err
			}
			i++
			switch a {
			case "--only":
				o.only, o.onlyGiven = split(v), true
			case "--skip":
				o.skip = split(v)
			case "--steps-dir":
				o.stepsDir = v
			default:
				o.ssh = append(o.ssh, a, v)
			}
		default:
			if strings.HasPrefix(a, "-") {
				return nil, fmt.Errorf("unknown option %s", a)
			}
			if o.host != "" {
				return nil, fmt.Errorf("unexpected argument %q (one host only)", a)
			}
			o.host = a
		}
	}
	return o, nil
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

func resolveStepsDir(o *options) string {
	switch {
	case o.stepsDir != "":
		return o.stepsDir
	case os.Getenv("SSS_STEPS_DIR") != "":
		return os.Getenv("SSS_STEPS_DIR")
	}
	return filepath.Join(exeDir(), "steps")
}

func readCommon(stepsDir string) (string, error) {
	for _, p := range []string{
		filepath.Join(stepsDir, "..", "lib", "common.sh"),
		filepath.Join(exeDir(), "lib", "common.sh"),
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("lib/common.sh not found next to the steps directory or the sss binary")
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

var unsafeRe = regexp.MustCompile(`[^A-Za-z0-9._@-]+`)

func fail(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "sss: "+format+"\n", a...)
	os.Exit(code)
}

func main() { os.Exit(run()) }

func run() int {
	o, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n\n%s", err, usage)
		return 2
	}
	stepsDir := resolveStepsDir(o)
	steps, err := LoadSteps(stepsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
		return 2
	}
	if o.list {
		for _, s := range steps {
			def := "off"
			if s.Default {
				def = "on"
			}
			hidden := ""
			if s.Hidden {
				hidden = " (hidden)"
			}
			fmt.Printf("%-20s default:%-3s requires:%-18s %s%s\n", s.ID, def, strings.Join(s.Requires, ","), s.Name, hidden)
		}
		return 0
	}
	if o.host == "" {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	common, err := readCommon(stepsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
		return 2
	}

	byID := stepIndex(steps)
	for _, id := range append(append([]string(nil), o.only...), o.skip...) {
		if byID[id] == nil {
			fmt.Fprintf(os.Stderr, "sss: unknown step %q (see sss --list)\n", id)
			return 2
		}
	}
	sel := LoadSelection(steps)
	if o.onlyGiven {
		for id := range sel {
			sel[id] = false
		}
		for _, id := range o.only {
			sel[id] = true
		}
	}
	for _, id := range o.skip {
		sel[id] = false
	}

	remote, err := NewRemote(o.host, o.ssh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
		return 2
	}
	defer remote.Close()
	fmt.Fprintf(os.Stderr, "Connecting to %s ...\n", o.host)
	if err := remote.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
		return 2
	}
	if err := remote.Preflight(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
		return 2
	}
	if osID := remote.Info.OS; osID != "debian" && osID != "ubuntu" {
		if !o.force {
			fmt.Fprintf(os.Stderr, "sss: unsupported OS %q (Debian and Ubuntu only; use --force to try anyway)\n", osID)
			return 2
		}
		fmt.Fprintf(os.Stderr, "sss: warning: unsupported OS %q, continuing because of --force\n", osID)
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".local", "state", "sss", "logs",
		unsafeRe.ReplaceAllString(o.host, "_"), time.Now().Format("20060102-150405"))
	runner := &Runner{Remote: remote, Common: common, LogDir: logDir}

	if o.yes || !isTTY(os.Stdout) || !isTTY(os.Stdin) {
		plan, err := Plan(steps, sel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sss: %v\n", err)
			return 2
		}
		if len(plan) == 0 {
			fmt.Fprintln(os.Stderr, "sss: nothing selected")
			return 2
		}
		fmt.Printf("Server: %s (%s)\n\n", o.host, remote.Info)
		return runPlain(os.Stdout, runner, plan, o.verbose)
	}

	m := newModel(o.host, remote.Info, steps, sel, runner, func(s map[string]bool) {
		if err := SaveSelection(steps, s); err != nil {
			fmt.Fprintf(os.Stderr, "sss: could not save selection: %v\n", err)
		}
	})
	code, err := runTUI(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sss: %v\n", err)
	}
	return code
}
