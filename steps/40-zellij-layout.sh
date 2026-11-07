#!/usr/bin/env bash
# sss-name: zellij compact layout
# sss-description: Compact layout (no bottom command bar), startup tips and release notes off
# sss-default: on
# sss-requires: zellij

cfg="$HOME/.config/zellij/config.kdl"
mkdir -p "$(dirname "$cfg")"
touch "$cfg"

# Comment out conflicting settings made elsewhere in the file (outside our block).
tmp="$tmpdir/config.kdl"
awk '
  /^\/\/ >>> sss layout >>>/ {inblk=1}
  { if (!inblk && $0 ~ /^[ \t]*(default_layout|show_startup_tips|show_release_notes)[ \t]/) print "// sss-disabled: " $0; else print }
  /^\/\/ <<< sss layout <<</ {inblk=0}
' "$cfg" > "$tmp"
cat "$tmp" > "$cfg"

# KDL comments use //, so build the block here instead of replace_block.
start='// >>> sss layout >>>'; end='// <<< sss layout <<<'
new=$(printf '%s\n%s\n%s\n%s\n%s\n' "$start" 'default_layout "compact"' 'show_startup_tips false' 'show_release_notes false' "$end")
old=$(awk -v s="$start" -v e="$end" '$0==s{f=1} f{print} $0==e{f=0}' "$cfg")
if [ "$old" = "$new" ]; then already_done "layout already configured"; fi
awk -v s="$start" -v e="$end" '$0==s{skip=1} !skip{print} $0==e{skip=0}' "$cfg" > "$tmp"
printf '%s\n' "$new" >> "$tmp"
cat "$tmp" > "$cfg"
log "wrote $cfg"
