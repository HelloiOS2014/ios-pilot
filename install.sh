#!/usr/bin/env bash
set -euo pipefail

# ios-pilot 安装脚本
#
# 远程一键安装（无需 Go）：
#   curl -sSL https://raw.githubusercontent.com/HelloiOS2014/ios-pilot/main/install.sh | bash
#
# 在仓库内运行：
#   ./install.sh
#
# 环境变量：
#   VERSION=v0.2.0        指定版本（默认最新）
#   INSTALL_SKILL=global   非交互 Skill 安装（global / skip / <项目路径>）

BINARY_NAME="ios-pilot"
INSTALL_DIR="${HOME}/.local/bin"
GLOBAL_SKILL_DIR="${HOME}/.claude/skills/ios-pilot"
GITHUB_REPO="HelloiOS2014/ios-pilot"

NEED_CLEANUP=""

# --- 颜色 ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }
step()  { echo -e "${CYAN}==>${NC} $1"; }

cleanup() {
    if [[ -n "$NEED_CLEANUP" ]] && [[ -d "$NEED_CLEANUP" ]]; then
        rm -rf "$NEED_CLEANUP"
    fi
}
trap cleanup EXIT

# --- 检查 macOS ---
check_os() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        error "ios-pilot 仅支持 macOS"
    fi
}

# --- 检测架构 ---
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64)  echo "amd64" ;;
        arm64)   echo "arm64" ;;
        *)       error "不支持的架构: ${arch}" ;;
    esac
}

# --- 获取最新版本 ---
get_latest_version() {
    local url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local tag
    tag=$(curl -sSL "$url" 2>/dev/null | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    if [[ -z "$tag" ]]; then
        return 1
    fi
    echo "$tag"
}

# --- 下载预编译二进制 ---
download_binary() {
    local version="$1"
    local arch="$2"
    local tmpdir="$3"

    local bin_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/ios-pilot-darwin-${arch}"
    local skill_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/SKILL.md"

    step "下载 ios-pilot ${version} (darwin/${arch})"

    if ! curl -sSL --fail -o "${tmpdir}/${BINARY_NAME}" "$bin_url" 2>/dev/null; then
        return 1
    fi
    chmod +x "${tmpdir}/${BINARY_NAME}"

    # 下载 SKILL.md
    curl -sSL --fail -o "${tmpdir}/SKILL.md" "$skill_url" 2>/dev/null || true

    info "下载完成"
    return 0
}

# --- 本地编译（fallback）---
build_from_source() {
    step "下载失败或无 Release，尝试本地编译"

    # 检查 Go
    if ! command -v go &>/dev/null; then
        error "无法下载预编译二进制，且 Go 未安装。请安装 Go 1.25+: https://go.dev/dl/"
    fi

    local root=""

    # 在仓库内？
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd 2>/dev/null || echo "")"
    if [[ -n "$script_dir" ]] && [[ -f "${script_dir}/go.mod" ]] && grep -q "module ios-pilot" "${script_dir}/go.mod" 2>/dev/null; then
        root="$script_dir"
    elif [[ -f "go.mod" ]] && grep -q "module ios-pilot" go.mod 2>/dev/null; then
        root="$(pwd)"
    else
        # clone
        info "克隆源码..." >&2
        local clone_dir
        clone_dir=$(mktemp -d)
        NEED_CLEANUP="$clone_dir"
        git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "${clone_dir}/ios-pilot" >&2 2>&1
        root="${clone_dir}/ios-pilot"
    fi

    info "编译中..." >&2
    local version="${VERSION:-dev}"
    (cd "$root" && go build -ldflags "-X ios-pilot/internal/cli.Version=${version}" -o "${BINARY_NAME}" ./cmd/ios-pilot/) 2>&1

    # 把产物和 SKILL.md 移到 tmpdir
    local tmpdir="$1"
    mv "${root}/${BINARY_NAME}" "${tmpdir}/${BINARY_NAME}"
    if [[ -f "${root}/skills/ios-pilot/SKILL.md" ]]; then
        cp "${root}/skills/ios-pilot/SKILL.md" "${tmpdir}/SKILL.md"
    fi

    info "编译完成"
}

# --- 安装二进制 ---
install_binary() {
    local tmpdir="$1"
    step "安装二进制"
    mkdir -p "${INSTALL_DIR}"
    cp "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    info "已安装到 ${INSTALL_DIR}/${BINARY_NAME}"

    if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
        warn "${INSTALL_DIR} 不在 PATH 中，请添加："
        echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
    fi
}

# --- 安装 Skill（交互式）---
install_skill() {
    local tmpdir="$1"
    local skill_src="${tmpdir}/SKILL.md"

    if [[ ! -f "$skill_src" ]]; then
        warn "SKILL.md 不可用，跳过 Skill 安装"
        return
    fi

    step "安装 Claude Code Skill"

    local choice="${INSTALL_SKILL:-}"

    # 非交互模式
    if [[ -n "$choice" ]]; then
        case "$choice" in
            global) do_install_skill "$skill_src" "$GLOBAL_SKILL_DIR" ;;
            skip)   info "跳过 Skill 安装" ;;
            *)      do_install_skill "$skill_src" "${choice}/.claude/skills/ios-pilot" ;;
        esac
        return
    fi

    # 交互模式
    echo ""
    echo "  Skill 安装方式："
    echo "    1) 全局安装（所有项目可用）"
    echo "    2) 安装到指定项目"
    echo "    3) 跳过"
    echo ""

    local input
    read -p "  请选择 [1]: " input </dev/tty || input="1"
    input="${input:-1}"

    case "$input" in
        1)
            do_install_skill "$skill_src" "$GLOBAL_SKILL_DIR"
            ;;
        2)
            echo ""
            local project_path
            read -p "  项目路径: " project_path </dev/tty || project_path=""
            if [[ -z "$project_path" ]]; then
                warn "未输入路径，跳过"
                return
            fi
            project_path="${project_path/#\~/$HOME}"
            if [[ ! -d "$project_path" ]]; then
                warn "目录不存在: ${project_path}"
                return
            fi
            do_install_skill "$skill_src" "${project_path}/.claude/skills/ios-pilot"
            ;;
        3)  info "跳过 Skill 安装" ;;
        *)  warn "无效选择，跳过" ;;
    esac
}

do_install_skill() {
    local src="$1"
    local dest_dir="$2"
    mkdir -p "$dest_dir"
    cp "$src" "${dest_dir}/SKILL.md"
    info "Skill 已安装到 ${dest_dir}/SKILL.md"
}

# --- 主流程 ---
main() {
    echo ""
    echo "  ios-pilot 安装"
    echo "  =============="
    echo ""

    check_os

    local arch
    arch=$(detect_arch)
    info "架构: darwin/${arch}"

    local tmpdir
    tmpdir=$(mktemp -d)
    # 确保 tmpdir 在所有情况下被清理
    local old_cleanup="$NEED_CLEANUP"
    NEED_CLEANUP="$tmpdir"

    # 确定版本
    local version="${VERSION:-}"
    if [[ -z "$version" ]]; then
        version=$(get_latest_version 2>/dev/null || echo "")
    fi

    local downloaded=false
    if [[ -n "$version" ]]; then
        if download_binary "$version" "$arch" "$tmpdir"; then
            downloaded=true
        fi
    fi

    if [[ "$downloaded" == "false" ]]; then
        build_from_source "$tmpdir"
    fi

    install_binary "$tmpdir"
    install_skill "$tmpdir"

    echo ""
    info "安装完成！"
    echo ""
    echo "  验证:  ${BINARY_NAME} --version"
    echo ""
    echo "  首次使用："
    echo "    1. USB 连接 iOS 设备并信任此电脑"
    echo "    2. ${BINARY_NAME} wda setup    # WDA 安装指南（仅首次）"
    echo "    3. ${BINARY_NAME} device connect"
    echo ""
}

main "$@"
