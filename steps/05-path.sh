#!/usr/bin/env bash
# sss-name: Add ~/.local/bin to PATH
# sss-description: For bash (~/.profile) and fish (conf.d); user-level installs live there
# sss-default: on

ensure_bin_dir
changed=0

replace_block "$HOME/.profile" path <<'BLK'
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) PATH="$HOME/.local/bin:$PATH" ;; esac
BLK
[ "$BLOCK_CHANGED" = 1 ] && changed=1

fish_conf="$HOME/.config/fish/conf.d/sss-path.fish"
mkdir -p "$(dirname "$fish_conf")"
want='fish_add_path -g $HOME/.local/bin'
if [ ! -f "$fish_conf" ] || [ "$(cat "$fish_conf")" != "$want" ]; then
  printf '%s\n' "$want" > "$fish_conf"
  changed=1
fi

if [ "$changed" = 0 ]; then already_done "PATH already configured"; fi
log "~/.local/bin is on PATH for bash and fish"
