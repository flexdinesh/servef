#!/bin/sh
# Packages the desktop app (decisions 0094, 0095) for one OS, per
# architecture, bundling the srcz binary of the matching CLI archive.
#
#   apps/desktop/scripts/package.sh linux|darwin VERSION ARCHIVES OUT [ARCHES]
#
# ARCHES is a space-separated list, amd64 and arm64 by default.
# ARCHIVES holds srcz_<VERSION>_<os>_<arch>.tar.gz (build.yml's cli job).
# Writes srcz-desktop_<VERSION>_linux_<arch>.tar.gz (mygo's archive) or
# srcz-desktop_<VERSION>_darwin_<arch>.zip (the ad-hoc signed app) to OUT.
# Runs from the repository root with Go 1.27 (apps/desktop/go.mod) and jq.
set -eu

os="$1"
version="$2"
archives="$(cd "$3" && pwd)"
mkdir -p "$4"
out="$(cd "$4" && pwd)"
arches="${5:-amd64 arm64}"
commit="$(git rev-parse --short=12 HEAD)"

# apps/desktop is its own module, outside go.work.
export GOWORK=off
cd apps/desktop
original="$(cat mygo.json)"
restore() {
  printf '%s\n' "$original" >mygo.json
  rm -rf build resources/bin
}
trap restore EXIT
config="$(jq --arg version "$version" '.version = $version' mygo.json)"
printf '%s\n' "$config" >mygo.json

for arch in $arches; do
  name="srcz_${version}_${os}_${arch}"
  tmp="$(mktemp -d)"
  tar -xzf "$archives/$name.tar.gz" -C "$tmp"
  mkdir -p resources/bin
  cp "$tmp/$name/srcz" resources/bin/srcz
  rm -rf "$tmp" build

  export MYGO_LDFLAGS="-X main.version=$version -X main.commit=$commit"
  if [ "$os" = linux ]; then
    go tool mygo build -platform "linux/$arch"
    mv "build/linux-$arch/srcz-desktop-$version-linux-$arch.tar.gz" "$out/srcz-desktop_${version}_linux_${arch}.tar.gz"
  else
    go tool mygo build -platform "darwin/$arch" -skip-dmg
    ditto -c -k --keepParent "build/darwin-$arch/srcz-desktop.app" "$out/srcz-desktop_${version}_darwin_${arch}.zip"
  fi
done
