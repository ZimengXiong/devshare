#!/bin/sh
set -eu

repo="${DEVSHARE_REPOSITORY:-ZimengXiong/devshare}"
version="${DEVSHARE_VERSION:-latest}"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset=devshare-linux-amd64 ;;
  Linux-aarch64|Linux-arm64) asset=devshare-linux-arm64 ;;
  Darwin-x86_64) asset=devshare-darwin-amd64 ;;
  Darwin-arm64) asset=devshare-darwin-arm64 ;;
  *) echo "Unsupported platform: $(uname -s) $(uname -m)" >&2; exit 1 ;;
esac

dest="${DEVSHARE_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dest"
if [ "$version" = latest ]; then
  url="https://github.com/$repo/releases/latest/download/$asset"
else
  url="https://github.com/$repo/releases/download/$version/$asset"
fi
curl -fL "$url" -o "$dest/devshare"
chmod 0755 "$dest/devshare"
echo "Installed $dest/devshare"
