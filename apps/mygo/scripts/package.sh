#!/bin/sh
# Packages the experimental mygo app (decision 0094) for one OS, per
# architecture, bundling the servef binary of the matching CLI archive.
#
#   apps/mygo/scripts/package.sh linux|darwin VERSION ARCHIVES OUT [ARCHES]
#
# ARCHES is a space-separated list, amd64 and arm64 by default.
# ARCHIVES holds servef_<VERSION>_<os>_<arch>.tar.gz (build.yml's cli job).
# Writes servef-mygo_<VERSION>_linux_<arch>.tar.gz (mygo's archive) or
# servef-mygo_<VERSION>_darwin_<arch>.zip (the ad-hoc signed app) to OUT.
# Runs from the repository root with Go 1.27 (apps/mygo/go.mod) and jq.
set -eu

os="$1"
version="$2"
archives="$(cd "$3" && pwd)"
mkdir -p "$4"
out="$(cd "$4" && pwd)"
arches="${5:-amd64 arm64}"
commit="$(git rev-parse --short=12 HEAD)"

# apps/mygo is its own module, outside go.work.
export GOWORK=off
cd apps/mygo
original="$(cat mygo.json)"
restore() {
  printf '%s\n' "$original" >mygo.json
  rm -rf build resources/bin
}
trap restore EXIT
config="$(jq --arg version "$version" '.version = $version' mygo.json)"
printf '%s\n' "$config" >mygo.json

for arch in $arches; do
  name="servef_${version}_${os}_${arch}"
  tmp="$(mktemp -d)"
  tar -xzf "$archives/$name.tar.gz" -C "$tmp"
  mkdir -p resources/bin
  cp "$tmp/$name/servef" resources/bin/servef
  rm -rf "$tmp" build

  export MYGO_LDFLAGS="-X main.version=$version -X main.commit=$commit"
  if [ "$os" = linux ]; then
    go tool mygo build -platform "linux/$arch"
    mv "build/linux-$arch/servef-mygo-$version-linux-$arch.tar.gz" "$out/servef-mygo_${version}_linux_${arch}.tar.gz"
  else
    go tool mygo build -platform "darwin/$arch" -skip-dmg
    ditto -c -k --keepParent "build/darwin-$arch/servef-mygo.app" "$out/servef-mygo_${version}_darwin_${arch}.zip"
  fi
done
