#!/usr/bin/env bash
# sss-name: Install Claude Code CLI
# sss-description: Official native installer (log in afterwards by running `claude`)
# sss-default: on
# sss-requires: base-packages

need_cmds curl
if have claude; then already_done "claude $(claude --version 2>/dev/null | head -n1) already installed"; fi
curl -fsSL https://claude.ai/install.sh | bash
have claude || [ -x "$HOME/.local/bin/claude" ] || die "installer finished but claude was not found in ~/.local/bin"
log "installed: $("$SSS_BIN/claude" --version 2>/dev/null | head -n1)"
