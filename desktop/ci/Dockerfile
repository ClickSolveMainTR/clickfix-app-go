# Masaüstü CI derleme konteyneri (.github/workflows/ci.yml, desktop işi).
# Self-hosted runner'a sistem paketi kurmadan Tauri'nin Linux bağımlılıklarını
# (webkit2gtk-4.1 vb.), Rust, Go, Node, pnpm ve golangci-lint'i sağlar.
# Sürümler: Rust (rust-version >= 1.90), Go = sidecar/go.mod, Node =
# frontend/.nvmrc, pnpm = package.json packageManager, golangci-lint =
# ci.yml GOLANGCI_LINT_VERSION. Birini değiştirirken burayı da güncelleyin.
FROM golang:1.26.11-trixie AS go
FROM node:22.22.0-trixie-slim AS node
FROM golangci/golangci-lint:v2.14.0 AS lint

FROM rust:1.97-trixie

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      build-essential file pkg-config \
      libwebkit2gtk-4.1-dev libgtk-3-dev libsoup-3.0-dev libjavascriptcoregtk-4.1-dev \
      libayatana-appindicator3-dev librsvg2-dev libssl-dev libxdo-dev \
 && rm -rf /var/lib/apt/lists/* \
 && rustup component add clippy rustfmt

COPY --from=go /usr/local/go /usr/local/go
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=lint /usr/bin/golangci-lint /usr/local/bin/golangci-lint

RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && npm install -g pnpm@11.22.0 \
 && npm cache clean --force

# Sürekli derleme önbellekleri /cache altına bağlanır (runner başına bir dizin).
# Konteyner runner kullanıcısının uid'siyle çalışır, kök sahipli dosya bırakmaz.
ENV PATH=/usr/local/go/bin:$PATH \
    GOTOOLCHAIN=local \
    HOME=/cache/home \
    CARGO_HOME=/cache/cargo \
    CARGO_TARGET_DIR=/cache/target \
    GOPATH=/cache/go \
    GOCACHE=/cache/go-build \
    GOLANGCI_LINT_CACHE=/cache/golangci-lint \
    XDG_CACHE_HOME=/cache/xdg \
    npm_config_store_dir=/cache/pnpm-store \
    CARGO_INCREMENTAL=0 \
    CARGO_PROFILE_DEV_DEBUG=0 \
    CI=true
