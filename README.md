# sss: ServerSetupScript

Set up a fresh Debian/Ubuntu server over SSH from a checklist of steps.

```
sss username@123.123.123.123
```

`sss` is a single Go binary that runs on your machine. It opens one SSH
connection (using your system `ssh`, so `~/.ssh/config`, agent, `ProxyJump`
and host aliases all work), shows a checklist, then runs each selected step on
the server and shows live progress and a per-step report.

## Install

```
make install        # builds, copies sss + steps/ + lib/ to ~/.local/share/sss, links ~/.local/bin/sss
```

`steps/` and `lib/` must stay next to the binary (or point `--steps-dir` /
`$SSS_STEPS_DIR` elsewhere; `lib/common.sh` is looked up at `<steps-dir>/../lib`).

## Usage

```
sss [options] [user@]host
  -y, --yes            skip the checklist; run the saved selection (plain output, no TTY needed)
      --only a,b       run only these step ids
      --skip a,b       leave these step ids out
      --steps-dir DIR  steps directory
  -p PORT / -i FILE / -o OPTION   passed to ssh (-o is repeatable)
  -v, --verbose        with --yes: print every output line too
      --list           list steps
```

Checklist keys: `↑/↓` move, `space` toggle, `a` all, `n` none, `enter` start, `q` quit.
Ticking a step also ticks what it requires; unticking a step unticks what depends on it.
When the run finishes, `r` retries only the failed steps (and the ones skipped because of them).

Your last confirmed checklist is saved to `~/.config/sss/selection` and is the
starting point next time. Logs go to `~/.local/state/sss/logs/<host>/<timestamp>/`.
Exit code: `0` all steps ok/already done/skipped, `1` at least one step failed, `2` usage or connection error.

### Requirements on the server

Debian or Ubuntu, x86_64 or arm64, and `bash`. Steps marked `needs-root` need
you to be root or have passwordless `sudo`. A preflight checks this once; without
root those steps are skipped, everything user-level still runs.

### Version pins

Tools install the latest release by default. Pin one for a run:

```
SSS_ZELLIJ_VERSION=0.43.1 SSS_HELIX_VERSION=25.07.1 SSS_YAZI_VERSION=25.5.31 sss user@host
```

## The built-in steps

| id | what it does |
|---|---|
| `base-packages` (hidden) | curl, unzip, xz, git, certificates; pulled in by the download steps |
| `path` | `~/.local/bin` on PATH for bash (`~/.profile`) and fish |
| `fish` | installs fish (apt) |
| `fish-default-shell` | adds fish to `/etc/shells`, sets it as your login shell |
| `zellij` | prebuilt binary into `~/.local/bin` |
| `zellij-layout` | `default_layout "compact"` (no bottom command bar), startup tips and release notes off |
| `zellij-autostart` | on interactive SSH logins, attach to the newest live zellij session or start one |
| `persistence` | logind `KillUserProcesses=no`, user lingering, sshd keepalives |
| `claude` | Claude Code via the official installer (run `claude` afterwards to log in) |
| `helix` | `hx` into `~/.local/bin`, runtime into `~/.config/helix/runtime` |
| `yazi` | `yazi` and `ya` into `~/.local/bin` |

### zellij on login

The login hook runs only for interactive SSH logins with a TTY, so `scp`,
`rsync`, `ssh host command` and cron are untouched. It does **not** `exec`
zellij, so detaching (`Ctrl+o d`) drops you at a normal fish prompt and the SSH
connection stays open. Skip it for one login with `NO_ZELLIJ=1`, e.g.
`ssh -t host 'NO_ZELLIJ=1 fish'`. Dead (EXITED) sessions are ignored, so you
never land on the "press ENTER to run" resurrection screen; "most recent" means
most recently *created* live session.

### Why sessions died on disconnect

The "press ENTER to run" screen is zellij's session *resurrection*: it only
appears after the old session's server process died. That usually means
systemd-logind killed your processes when the SSH session ended. The
`persistence` step sets `KillUserProcesses=no`, enables lingering and sets
sshd `ClientAliveInterval 30` / `ClientAliveCountMax 6` (dead clients are dropped
after about 3 minutes). It verifies the effective logind setting afterwards, but
a real disconnect test (close the lid, reconnect) is up to you. If it still
fails, the fallback is running the zellij server as a systemd user service.

## Writing a step

A step is `steps/NN-id.sh`. `NN` sets the order, `id` is what `--only` and
`requires` use. Put `# sss-` headers at the top (all optional):

```bash
#!/usr/bin/env bash
# sss-name: Install Codex
# sss-description: Shown under the cursor in the checklist
# sss-default: on            # on|off, start ticked or not
# sss-requires: base-packages  # comma-separated step ids, run first, ticked automatically
# sss-needs-root: false      # skipped when the login has no root/sudo
# sss-hidden: false          # true = not in the checklist, only pulled in via requires
# sss-timeout: 600           # seconds, default 600

need_cmds curl
if have codex; then already_done "codex already installed"; fi
# ... install ...
log "installed codex"
```

Exit codes: `0` success, `10` already done (nothing changed), `11` not
applicable here (shown as skipped), anything else is a failure. The last output
line becomes the one-line result; on failure the last lines are shown as the
error. Write steps so re-running is safe.

`lib/common.sh` is sent ahead of every step and runs with `set -eu`. Helpers:
`log`, `die`, `have`, `already_done`, `not_applicable`, `as_root`, `apt_install`,
`need_cmds`, `ensure_bin_dir`, `resolve_tag NAME owner/repo` (latest GitHub
release, honouring `SSS_<NAME>_VERSION`), `download URL DEST`,
`replace_block FILE MARKER <<EOF` (idempotent managed block, sets
`BLOCK_CHANGED`), `$tmpdir` (removed on exit). Variables: `$SSS_ARCH`
(`x86_64`|`aarch64`), `$SSS_ROOT` (`root`|`sudo`|`none`), `$SSS_BIN` (`~/.local/bin`).

To remove a step, delete its file. To change one, edit it; nothing needs rebuilding.

## Testing

`test/run.sh` builds an Ubuntu (or Debian) container with sshd and runs `sss`
against it. Variants: `PLATFORM=linux/amd64`, `SSS_USER=plain` (no sudo),
`BASE=debian:12`, `KEEP=1`. The container has no systemd, so the `persistence`
step reports "skipped" there.
