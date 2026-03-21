#!/usr/bin/env bash
set -euo pipefail

# ios-pilot 安装脚本
#
# 在仓库内运行：
#   ./install.sh
#
# 远程一键安装：
#   curl -sSL https://raw.githubusercontent.com/<owner>/ios-pilot/main/install.sh | bash
#
# 非交互模式（CI 等场景）：
#   INSTALL_SKILL=global ./install.sh      # Skill 装全局
#   INSTALL_SKILL=skip ./install.sh        # 跳过 Skill
#   INSTALL_SKILL=/path/to/project ./install.sh  # 装到指定项目

BINARY_NAME="ios-pilot"
INSTALL_DIR="${HOME}/.local/bin"
GLOBAL_SKILL_DIR="${HOME}/.claude/skills/ios-pilot"
REPO_URL="${IOS_PILOT_REPO:-https://github.com/user/ios-pilot.git}"
MIN_GO_VERSION="1.25"

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

# --- 清理 ---
cleanup() {
    if [[ -n "$NEED_CLEANUP" ]] && [[ -d "$NEED_CLEANUP" ]]; then
        rm -rf "$NEED_CLEANUP"
    fi
}
trap cleanup EXIT

# --- 1. 检查前置条件 ---
check_prerequisites() {
    step "检查前置条件"

    # macOS
    if [[ "$(uname -s)" != "Darwin" ]]; then
        error "ios-pilot 仅支持 macOS"
    fi

    # Go
    if ! command -v go &>/dev/null; then
        error "Go 未安装。请先安装 Go ${MIN_GO_VERSION}+: https://go.dev/dl/"
    fi
    local ver
    ver=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
    local major minor
    major=$(echo "$ver" | cut -d. -f1)
    minor=$(echo "$ver" | cut -d. -f2)
    if [[ "$major" -lt 1 ]] || { [[ "$major" -eq 1 ]] && [[ "$minor" -lt 25 ]]; }; then
        error "Go 版本太低（${ver}），需要 ${MIN_GO_VERSION}+。"
    fi
    info "Go ${ver}"

    # git（远程安装需要）
    if ! command -v git &>/dev/null; then
        error "git 未安装"
    fi
}

# --- 2. 获取源码 ---
get_source() {
    step "定位源码" >&2

    # 优先检查脚本所在目录
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd 2>/dev/null || echo "")"

    if [[ -n "$script_dir" ]] && [[ -f "${script_dir}/go.mod" ]] && grep -q "module ios-pilot" "${script_dir}/go.mod" 2>/dev/null; then
        info "使用本地源码: ${script_dir}" >&2
        echo "$script_dir"
        return
    fi

    # 当前目录
    if [[ -f "go.mod" ]] && grep -q "module ios-pilot" go.mod 2>/dev/null; then
        info "使用本地源码: $(pwd)" >&2
        echo "$(pwd)"
        return
    fi

    # 远程 clone
    info "远程模式，克隆源码..." >&2
    local tmpdir
    tmpdir=$(mktemp -d)
    NEED_CLEANUP="$tmpdir"
    git clone --depth 1 "$REPO_URL" "${tmpdir}/ios-pilot" >&2 2>&1
    info "源码已克隆到临时目录" >&2
    echo "${tmpdir}/ios-pilot"
}

# --- 3. 编译 ---
build_binary() {
    local root="$1"
    step "编译 ${BINARY_NAME}"
    (cd "$root" && go build -o "${BINARY_NAME}" ./cmd/ios-pilot/) 2>&1
    info "编译完成"
}

# --- 4. 安装二进制 ---
install_binary() {
    local root="$1"
    step "安装二进制"
    mkdir -p "${INSTALL_DIR}"
    cp "${root}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    # 清理编译产物
    rm -f "${root}/${BINARY_NAME}"
    info "已安装到 ${INSTALL_DIR}/${BINARY_NAME}"

    if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
        warn "${INSTALL_DIR} 不在 PATH 中，请添加："
        echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
    fi
}

# --- 5. 安装 Skill（交互式） ---
install_skill() {
    local root="$1"
    local skill_src="${root}/skills/ios-pilot/SKILL.md"

    if [[ ! -f "$skill_src" ]]; then
        warn "找不到 SKILL.md，跳过 Skill 安装"
        return
    fi

    step "安装 Claude Code Skill"

    local choice="${INSTALL_SKILL:-}"

    # 非交互模式
    if [[ -n "$choice" ]]; then
        case "$choice" in
            global)
                do_install_skill "$skill_src" "$GLOBAL_SKILL_DIR"
                ;;
            skip)
                info "跳过 Skill 安装"
                ;;
            *)
                # 视为项目路径
                do_install_skill "$skill_src" "${choice}/.claude/skills/ios-pilot"
                ;;
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
                warn "未输入路径，跳过 Skill 安装"
                return
            fi
            # 展开 ~
            project_path="${project_path/#\~/$HOME}"
            if [[ ! -d "$project_path" ]]; then
                warn "目录不存在: ${project_path}，跳过 Skill 安装"
                return
            fi
            do_install_skill "$skill_src" "${project_path}/.claude/skills/ios-pilot"
            ;;
        3)
            info "跳过 Skill 安装"
            ;;
        *)
            warn "无效选择，跳过 Skill 安装"
            ;;
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

    check_prerequisites

    local root
    root=$(get_source)

    build_binary "$root"
    install_binary "$root"
    install_skill "$root"

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
