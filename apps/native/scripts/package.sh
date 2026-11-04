#!/bin/sh
# Packages the experimental native app (decision 0082) for one OS, both
# architectures, bundling the servef binary of the matching CLI archive.
#
#   apps/native/scripts/package.sh linux|darwin VERSION ARCHIVES OUT
#
# ARCHIVES holds servef_<VERSION>_<os>_<arch>.tar.gz (build.yml's cli job).
# Writes servef-native_<VERSION>_linux_<arch>.tar.gz (mygo's archive) or
# servef-native_<VERSION>_darwin_<arch>.zip (the ad-hoc signed app) to OUT,
# each with THIRD_PARTY_NOTICES.txt (scripts/notices.sh) in its resources.
# Runs from the repository root with Go 1.27 (apps/native/go.mod) and jq.
set -eu

os="$1"
version="$2"
archives="$(cd "$3" && pwd)"
mkdir -p "$4"
out="$(cd "$4" && pwd)"
commit="$(git rev-parse --short=12 HEAD)"

# apps/native is its own module, outside go.work.
export GOWORK=off
cd apps/native
original="$(cat mygo.json)"
restore() {
  printf '%s\n' "$original" >mygo.json
  rm -rf build resources/bin resources/THIRD_PARTY_NOTICES.txt
}
trap restore EXIT
config="$(jq --arg version "$version" '.version = $version' mygo.json)"
printf '%s\n' "$config" >mygo.json
# Licenses of the embedded wasm modules, wazero, and Pierre's tree icons,
# in every package.
sh scripts/notices.sh resources/THIRD_PARTY_NOTICES.txt

for arch in amd64 arm64; do
  name="servef_${version}_${os}_${arch}"
  tmp="$(mktemp -d)"
  tar -xzf "$archives/$name.tar.gz" -C "$tmp"
  mkdir -p resources/bin
  cp "$tmp/$name/servef" resources/bin/servef
  rm -rf "$tmp" build

  export MYGO_LDFLAGS="-X main.version=$version -X main.commit=$commit"
  if [ "$os" = linux ]; then
    go tool mygo build -platform "linux/$arch"
    mv "build/linux-$arch/servef-native-$version-linux-$arch.tar.gz" "$out/servef-native_${version}_linux_${arch}.tar.gz"
  else
    go tool mygo build -platform "darwin/$arch" -skip-dmg
    ditto -c -k --keepParent "build/darwin-$arch/servef-native.app" "$out/servef-native_${version}_darwin_${arch}.zip"
  fi
done
