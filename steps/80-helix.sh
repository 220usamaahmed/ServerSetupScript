#!/usr/bin/env bash
# sss-name: Install helix
# sss-description: Helix editor (hx) and its runtime, into ~/.local/bin and ~/.config/helix (pin: SSS_HELIX_VERSION)
# sss-default: on
# sss-requires: base-packages

need_cmds curl tar xz
ensure_bin_dir
tag=$(resolve_tag HELIX helix-editor/helix)
want="${tag#v}"
if have hx && hx --version | head -n1 | grep -qF "helix $want"; then
  already_done "helix $want already installed"
fi
download "https://github.com/helix-editor/helix/releases/download/$tag/helix-$want-$SSS_ARCH-linux.tar.xz" "$tmpdir/hx.txz"
tar -xJf "$tmpdir/hx.txz" -C "$tmpdir"
src=$(find "$tmpdir" -maxdepth 1 -type d -name 'helix-*' | head -n1)
[ -x "$src/hx" ] || die "hx binary not found in the archive"
install -m 0755 "$src/hx" "$SSS_BIN/hx"
rm -rf "$HOME/.config/helix/runtime"
mkdir -p "$HOME/.config/helix"
cp -r "$src/runtime" "$HOME/.config/helix/runtime"
log "installed $("$SSS_BIN/hx" --version | head -n1)"
