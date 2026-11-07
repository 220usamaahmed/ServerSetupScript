#!/usr/bin/env bash
# sss-name: zellij on SSH login
# sss-description: Attach to the most recent zellij session (or start one) on interactive SSH logins only
# sss-default: on
# sss-requires: zellij

ensure_bin_dir
changed=0
helper="$SSS_BIN/sss-zellij-attach"
cat > "$tmpdir/helper" <<'HELPER'
#!/bin/sh
# Attach to the most recently created live zellij session; start a new one if none.
# Dead (EXITED) sessions are ignored so you never land on the "press ENTER to run" screen.
command -v zellij >/dev/null 2>&1 || exit 0
# list-sessions prints oldest first, so the last live line is the newest session.
live=$(zellij list-sessions --no-formatting 2>/dev/null | grep -v EXITED | sed 's/ \[Created.*//' | tail -n 1)
if [ -n "$live" ]; then exec zellij attach "$live"; fi
exec zellij
HELPER
if ! cmp -s "$tmpdir/helper" "$helper" 2>/dev/null; then
  install -m 0755 "$tmpdir/helper" "$helper"; changed=1
fi

# fish
fish_conf="$HOME/.config/fish/conf.d/sss-zellij.fish"
mkdir -p "$(dirname "$fish_conf")"
cat > "$tmpdir/fish" <<'FISH'
# Managed by sss. Interactive SSH logins only; scp/rsync/ssh host cmd are unaffected.
# Skip with:  NO_ZELLIJ=1 ssh -t host
if status is-interactive; and set -q SSH_TTY; and not set -q ZELLIJ; and not set -q NO_ZELLIJ; and test -x $HOME/.local/bin/sss-zellij-attach
    $HOME/.local/bin/sss-zellij-attach
end
FISH
if ! cmp -s "$tmpdir/fish" "$fish_conf" 2>/dev/null; then cp "$tmpdir/fish" "$fish_conf"; changed=1; fi

# bash (used when fish is not the login shell)
replace_block "$HOME/.bashrc" zellij <<'BASHRC'
if [[ $- == *i* ]] && [ -n "${SSH_TTY:-}" ] && [ -z "${ZELLIJ:-}" ] && [ -z "${NO_ZELLIJ:-}" ] && [ -x "$HOME/.local/bin/sss-zellij-attach" ]; then
  "$HOME/.local/bin/sss-zellij-attach"
fi
BASHRC
[ "$BLOCK_CHANGED" = 1 ] && changed=1

if [ "$changed" = 0 ]; then already_done "autostart already configured"; fi
log "zellij will start on interactive SSH logins (skip with NO_ZELLIJ=1)"
