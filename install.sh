#!/bin/sh
# Terra-Prism Installer
# Usage: curl -sSfL https://raw.githubusercontent.com/CaptShanks/terraprism/main/install.sh | sh

set -e

REPO="CaptShanks/terraprism"
BINARY_NAME="terraprism"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}info:${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}success:${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}warning:${NC} %s\n" "$1"
}

error() {
    printf "${RED}error:${NC} %s\n" "$1"
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     OS="linux";;
        Darwin*)    OS="darwin";;
        MINGW*|MSYS*|CYGWIN*) OS="windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   ARCH="amd64";;
        aarch64|arm64)  ARCH="arm64";;
        *)              error "Unsupported architecture: $(uname -m)";;
    esac
}

# Get latest version from GitHub
get_latest_version() {
    VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        # If no releases yet, use main branch
        VERSION="main"
        warn "No releases found, will build from source"
        return 1
    fi
    return 0
}

# Download and install binary
install_binary() {
    local version="$1"
    local os="$2"
    local arch="$3"
    
    local filename="${BINARY_NAME}-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        filename="${filename}.exe"
    fi
    
    local url="https://github.com/${REPO}/releases/download/${version}/${filename}"
    
    info "Downloading ${BINARY_NAME} ${version} for ${os}/${arch}..."
    
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT
    
    if ! curl -sSfL "$url" -o "${TMP_DIR}/${BINARY_NAME}"; then
        return 1
    fi
    
    chmod +x "${TMP_DIR}/${BINARY_NAME}"
    
    # Install
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        info "Installing to ${INSTALL_DIR} requires sudo..."
        sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi
    
    return 0
}

# Install from source using Go
install_from_source() {
    info "Installing from source..."
    
    if ! command -v go >/dev/null 2>&1; then
        error "Go is required to install from source. Install Go from https://go.dev"
    fi
    
    go install "github.com/${REPO}/cmd/terraprism@latest"
    
    # Check if GOPATH/bin is in PATH
    GOBIN="${GOPATH:-$HOME/go}/bin"
    if ! echo "$PATH" | grep -q "$GOBIN"; then
        warn "Add ${GOBIN} to your PATH:"
        echo "  export PATH=\"\$PATH:${GOBIN}\""
    fi
}

main() {
    echo ""
    echo "🔺 Terra-Prism Installer"
    echo "========================"
    echo ""
    
    detect_os
    detect_arch
    
    info "Detected: ${OS}/${ARCH}"
    
    if get_latest_version; then
        info "Latest version: ${VERSION}"
        
        if install_binary "$VERSION" "$OS" "$ARCH"; then
            success "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
            echo ""
            echo "Run 'terraprism --help' to get started!"
            echo ""
            exit 0
        else
            warn "Binary download failed, falling back to source install..."
        fi
    fi
    
    # Fallback to source install
    install_from_source
    success "Installed ${BINARY_NAME} via 'go install'"
    echo ""
    echo "Run 'terraprism --help' to get started!"
    echo ""
}

main "$@"

