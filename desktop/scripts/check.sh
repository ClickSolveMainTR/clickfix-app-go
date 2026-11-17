#!/usr/bin/env bash
# Masaüstü çalışma alanının tüm denetimleri. `make desktop-check` ve CI
# (desktop işi, ci/Dockerfile konteyneri) aynı betiği çalıştırır.
#
#   DESKTOP_TAURI_BUILD=1  ek olarak iki uygulama için `tauri build --no-bundle`
#                          (yavaş; CI açar, yerelde isteğe bağlı)
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
pnpm="${PNPM:-pnpm}"
step() { printf '\n==> %s\n' "$*"; }

step "Sürüm tutarlılığı (VERSION)"
version="$(tr -d '[:space:]' < VERSION)"
fail=0
check_version() {
  if [ "$2" != "$version" ]; then
    echo "$1: $2, beklenen $version (desktop/VERSION)"; fail=1
  fi
}
check_version Cargo.toml "$(sed -n 's/^version = "\(.*\)"/\1/p' Cargo.toml | head -1)"
for f in apps/agent/src-tauri/tauri.conf.json apps/support/src-tauri/tauri.conf.json \
         apps/agent/package.json apps/support/package.json; do
  check_version "$f" "$(node -p "require('./$f').version")"
done
[ "$fail" = 0 ] || exit 1
echo "$version"

step "Sidecar: gofmt, go vet, go test, golangci-lint"
(
  cd sidecar
  unformatted="$(gofmt -l .)"
  if [ -n "$unformatted" ]; then echo "gofmt gerekli: $unformatted"; exit 1; fi
  go vet ./...
  go test -count=1 ./...
  if command -v golangci-lint >/dev/null; then
    golangci-lint run ./...
  elif [ -n "${CI:-}" ]; then
    echo "golangci-lint yok"; exit 1
  else
    echo "golangci-lint yok, yerelde atlandı"
  fi
)

step "Sidecar ikilisi (bundle.externalBin)"
node scripts/build-sidecar.mjs
host="$(rustc -vV | sed -n 's/^host: //p')"
ext=""; case "$host" in *-windows-*) ext=".exe" ;; esac
export CLICKFIX_SIDECAR_BIN="$root/apps/support/src-tauri/binaries/clickfix-sidecar-$host$ext"

step "Destek arayüzü: install, typecheck, lint, build"
"$pnpm" install --frozen-lockfile
"$pnpm" --filter @clickfix-desktop/support typecheck
"$pnpm" --filter @clickfix-desktop/support lint
"$pnpm" --filter @clickfix-desktop/support build

step "Rust: fmt, clippy, test"
cargo fmt --all --check
cargo clippy --workspace --all-targets --locked -- -D warnings
cargo test --workspace --locked

if [ "${DESKTOP_TAURI_BUILD:-}" = "1" ]; then
  for app in agent support; do
    step "tauri build --no-bundle ($app)"
    "$pnpm" --filter "@clickfix-desktop/$app" tauri build --no-bundle --ci
  done
fi

step "desktop-check geçti"
