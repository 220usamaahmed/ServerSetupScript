#!/usr/bin/env bash
# sss-name: Make fish the default shell
# sss-description: Registers fish in /etc/shells and runs chsh for your user
# sss-default: on
# sss-requires: fish
# sss-needs-root: true

fish_path=$(command -v fish) || die "fish is not installed"
me=$(id -un)
current=$(getent passwd "$me" | cut -d: -f7)
if [ "$current" = "$fish_path" ]; then already_done "login shell is already $fish_path"; fi

grep -qxF "$fish_path" /etc/shells || { printf '%s\n' "$fish_path" | as_root tee -a /etc/shells >/dev/null; }
as_root usermod -s "$fish_path" "$me"
log "login shell for $me changed from $current to $fish_path"
