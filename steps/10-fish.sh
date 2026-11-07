#!/usr/bin/env bash
# sss-name: Install fish
# sss-description: The fish shell, from the distro package (apt)
# sss-default: on
# sss-needs-root: true

if have fish; then already_done "fish $(fish --version | awk '{print $NF}') already installed"; fi
apt_install fish
log "installed $(fish --version)"
