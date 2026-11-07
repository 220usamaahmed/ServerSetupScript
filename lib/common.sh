# lib/common.sh -- shared helpers, sent to the server ahead of every step.
# sss prepends these variables (see remote.go):
#   SSS_ARCH   x86_64 | aarch64
#   SSS_ROOT   root | sudo | none
# Exit codes a step may use:
#   0   success (something changed)
#   10  already done, nothing changed
#   11  not applicable on this server (shown as skipped)
#   *   failure; the last output lines become the error message

set -eu
if [ -n "${BASH_VERSION:-}" ]; then set -o pipefail; fi

SSS_BIN="$HOME/.local/bin"
export PATH="$SSS_BIN:$PATH"
export DEBIAN_FRONTEND=noninteractive

log()  { printf '%s\n' "$*"; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

already_done()   { log "${*:-already done}"; exit 10; }
not_applicable() { log "${*:-not applicable}"; exit 11; }

# as_root cmd args...  -- run a command as root (directly, or via sudo).
# With SSS_ROOT=sudo-password, sudo asks for the password through a tiny
# askpass script that prints $SSS_SUDO_PW (set by sss; never on a command line).
as_root() {
  case "$SSS_ROOT" in
    root) "$@" ;;
    sudo) sudo -n "$@" ;;
    sudo-password)
      [ -n "${SSS_SUDO_PW:-}" ] || die "this needs your sudo password, which was not provided"
      if [ ! -x "$tmpdir/askpass" ]; then
        cat > "$tmpdir/askpass" <<'ASKPASS'
#!/bin/sh
printf '%s\n' "$SSS_SUDO_PW"
ASKPASS
        chmod 700 "$tmpdir/askpass"
      fi
      SUDO_ASKPASS="$tmpdir/askpass" sudo -A "$@" ;;
    *)    die "this needs root or passwordless sudo, and neither is available" ;;
  esac
}

# apt_install pkg...
apt_install() {
  as_root apt-get update -qq
  as_root apt-get install -y -qq --no-install-recommends "$@"
}

ensure_bin_dir() { mkdir -p "$SSS_BIN"; }

# latest_tag owner/repo  -> prints the tag of the latest GitHub release.
# Uses the releases/latest redirect, so there is no API rate limit.
latest_tag() {
  local url
  url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$1/releases/latest") \
    || die "could not look up the latest release of $1"
  printf '%s\n' "${url##*/}"
}

# resolve_tag NAME owner/repo -> honours SSS_<NAME>_VERSION as a pin, else latest.
# NAME is upper-case, e.g. ZELLIJ. A pin may be given with or without a leading v;
# the result is the tag as it appears in GitHub, found by probing both spellings.
resolve_tag() {
  local name="$1" repo="$2" pin
  eval "pin=\${SSS_${name}_VERSION:-}"
  if [ -z "$pin" ]; then latest_tag "$repo"; return; fi
  pin="${pin#v}"
  if curl -fsI -o /dev/null "https://github.com/$repo/releases/tag/v$pin"; then
    printf 'v%s\n' "$pin"
  else
    printf '%s\n' "$pin"
  fi
}

# download URL DEST
download() {
  log "downloading $1"
  curl -fL --retry 3 --retry-delay 2 -sS -o "$2" "$1" || die "download failed: $1"
}

# need_cmds cmd... -- fail clearly if prerequisites are missing
need_cmds() {
  local missing="" c
  for c in "$@"; do have "$c" || missing="$missing $c"; done
  [ -z "$missing" ] || die "missing required commands:$missing (enable the base-packages step)"
}

# scratch dir that is removed when the step ends
tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/sss.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT

# replace_block FILE MARKER  (content on stdin)
# Idempotently maintains a block delimited by "# >>> sss MARKER >>>" lines.
# Sets BLOCK_CHANGED=1 if the file was modified, 0 if it already matched.
BLOCK_CHANGED=0
replace_block() {
  local file="$1" marker="$2" start end new old
  start="# >>> sss $marker >>>"
  end="# <<< sss $marker <<<"
  new=$(printf '%s\n' "$start"; cat; printf '%s\n' "$end")
  mkdir -p "$(dirname "$file")"; touch "$file"
  old=$(awk -v s="$start" -v e="$end" '$0==s{f=1} f{print} $0==e{f=0}' "$file")
  if [ "$old" = "$new" ]; then BLOCK_CHANGED=0; return 0; fi
  awk -v s="$start" -v e="$end" '$0==s{skip=1} !skip{print} $0==e{skip=0}' "$file" > "$tmpdir/rb.out"
  printf '%s\n' "$new" >> "$tmpdir/rb.out"
  cat "$tmpdir/rb.out" > "$file"
  BLOCK_CHANGED=1
}
