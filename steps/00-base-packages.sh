#!/usr/bin/env bash
# sss-name: Base packages
# sss-description: curl, unzip, xz, git and certificates needed by the download steps
# sss-default: on
# sss-hidden: true
# sss-needs-root: optional

pkgs="curl ca-certificates unzip xz-utils file git tar"
missing=""
for p in curl unzip xz file git tar; do have "$p" || missing="$missing $p"; done
[ -f /etc/ssl/certs/ca-certificates.crt ] || missing="$missing ca-certificates"
if [ -z "$missing" ]; then already_done "all base packages present"; fi

if [ "$SSS_ROOT" = none ]; then die "missing:$missing, and no root or sudo is available to install them"; fi
log "installing $pkgs"
apt_install $pkgs
log "installed: $pkgs"
