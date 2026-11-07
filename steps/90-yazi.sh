#!/usr/bin/env bash
# sss-name: Install yazi
# sss-description: Terminal file manager (yazi, ya) into ~/.local/bin (pin: SSS_YAZI_VERSION)
# sss-default: on
# sss-requires: base-packages

need_cmds curl unzip
ensure_bin_dir
tag=$(resolve_tag YAZI sxyazi/yazi)
want="${tag#v}"
if have yazi && yazi --version | grep -qF "$want"; then
  already_done "yazi $want already installed"
fi
asset="yazi-$SSS_ARCH-unknown-linux-musl"
download "https://github.com/sxyazi/yazi/releases/download/$tag/$asset.zip" "$tmpdir/yazi.zip"
unzip -q -o "$tmpdir/yazi.zip" -d "$tmpdir"
[ -x "$tmpdir/$asset/yazi" ] || die "yazi binary not found in the archive"
install -m 0755 "$tmpdir/$asset/yazi" "$SSS_BIN/yazi"
[ -x "$tmpdir/$asset/ya" ] && install -m 0755 "$tmpdir/$asset/ya" "$SSS_BIN/ya"
log "installed yazi $("$SSS_BIN/yazi" --version | awk '/Version/{print $2}')"
