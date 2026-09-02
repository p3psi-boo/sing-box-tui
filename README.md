# sing-box-tui

面向 [sing-box](https://github.com/SagerNet/sing-box) API 的终端用户界面。

## 简介

sing-box-tui 是一个通过 sing-box 管理 API 对实例进行管理的 TUI 客户端。
它与 [sing-box dashboard](https://github.com/SagerNet/sing-box-dashboard)
Web 应用承担相同职能，区别在于它运行在终端内，既不依赖浏览器，也不需要本地
Web 服务。目标使用场景是运维人员在终端会话中管理 sing-box，包括只能通过
SSH 访问的无头主机。

范围与约束：

- 仅作为客户端。sing-box-tui 从服务器暴露的 API 读取状态并发送控制指令，
  不内嵌、不配置、不重启 sing-box。
- 支持两种 API 面：Clash API（sing-box ≥ 1.10.0）与 gRPC API 服务
  （sing-box ≥ 1.14.0）。连接时自动探测 API 类型，优先探测 Clash API。
- sing-box 服务无需运行在本机。远端主机可直接经网络访问，或通过客户端
  建立的 SSH 隧道访问。

## 使用教程

### 1. 前置条件

一个启用了管理 API 的 sing-box 实例。API 自动探测优先尝试 Clash API，
失败后回退到 gRPC API 服务。

**Clash API**（sing-box 1.10.0+）：

```json
{
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "secret": "your-secret"
    }
  }
}
```

**sing-box API**（sing-box 1.14.0+）：

```json
{
  "services": [
    {
      "type": "api",
      "listen": "0.0.0.0",
      "listen_port": 9090,
      "secret": "your-secret"
    }
  ]
}
```

### 2. 构建

```bash
make generate   # 重新生成 protobuf 绑定（buf generate）
make build      # 构建 bin/sing-box-tui
```

需要 Go 1.23+、`buf` 与 `protoc-gen-go`。NixOS 下 `nix develop` 可提供完整
工具链。

### 3. 配置

默认配置路径：`~/.config/sing-box-tui/config.yaml`。可通过
`-config <path>` 覆盖。

```yaml
active: home
servers:
  - id: home
    name: Home Router
    type: direct
    api: auto            # auto | clash | grpc
    address: 192.168.1.1:9090
    secret: your-secret

  - id: vps
    name: VPS via SSH
    type: ssh
    secret: your-secret
    ssh:
      host: vps.example.com
      port: 22
      user: root
      identity_file: ~/.ssh/id_ed25519
    remote_address: 127.0.0.1:9090
```

字段说明：

| 字段             | 类型   | 说明                                            |
|------------------|--------|-------------------------------------------------|
| `id`             | string | 服务器唯一标识                                  |
| `name`           | string | 显示名称                                        |
| `type`           | string | `direct` 或 `ssh`                               |
| `api`            | string | `auto`（默认）、`clash` 或 `grpc`               |
| `address`        | string | API 端点（`host:port`）                          |
| `secret`         | string | API 密钥；空字符串表示无密钥                     |
| `tls`            | bool   | 直连使用 HTTPS（仅 Clash API 路径）              |
| `ssh.*`          | —      | SSH 隧道参数（`host`、`port`、`user`、`identity_file`、`known_hosts_file`） |
| `remote_address` | string | 从 SSH 主机视角看到的 API 端点                   |

### 4. 运行

```bash
./bin/sing-box-tui                  # 启动 TUI
./bin/sing-box-tui connect          # 连通性检查，不起 TUI
./bin/sing-box-tui connect home     # 对指定服务器做连通性检查
```

`connect` 输出服务器版本与 API 版本后退出，适合在进入交互界面之前验证配置。

### 5. 快捷键

**全局**

| 按键 | 操作 |
|------|------|
| `1` / `2` / `3` | 分组 / 连接 / 日志 |
| `Tab` / `Shift+Tab` | 下一页 / 上一页 |
| `j` / `k` | 下移 / 上移 |
| `[` / `]` | 切换 Clash 模式 |
| `s` | 服务器 |
| `r` | 重连 |
| `?` | 帮助 |
| `q` | 退出 |

**分组**

| 按键 | 操作 |
|------|------|
| `Enter` | 展开分组或选择出站 |
| `e` | 展开 / 收起 |
| `t` | URL 测速 |

**连接**

| 按键 | 操作 |
|------|------|
| `/` | 搜索（Enter 保留过滤条件，Esc 清除） |
| `f` | 活跃 / 全部 / 已关闭 |
| `x` | 关闭连接 |
| `D` | 全部关闭（需确认） |

**日志**

| 按键 | 操作 |
|------|------|
| `f` | 循环切换日志级别（当前级别及更严重） |
| `c` | 清空日志 |

**服务器**（`s`）

| 按键 | 操作 |
|------|------|
| `Enter` | 切换服务器 |
| `a` | 添加服务器 |
| `d` | 删除服务器（需确认） |

## 能力

- **多主机管理** —— 单个配置可包含任意数量的服务器；每台服务器独立指定
  连接类型（`direct` / `ssh`）、API 偏好与密钥。
- **状态栏** —— 服务器可达性、Clash 模式、实时流量、运行时长。
- **代理分组** —— 展开 / 收起分组、查看出站、切换选择器、执行 URL 测速。
- **连接** —— 实时连接列表，支持搜索、状态过滤（活跃 / 全部 / 已关闭）
  与单条关闭。依赖服务器上的 Clash API。
- **日志** —— 流式日志视图，支持日志级别过滤。
- **服务器切换** —— 会话内切换活跃服务器；可在 TUI 内添加、删除服务器
  （变更后重写配置文件）。
- **API 兼容性** —— Clash API（sing-box 1.10.0+）与 gRPC API 服务
  （sing-box 1.14.0+）。

已知限制：

- 连接视图依赖 Clash API。gRPC 面目前不提供连接列表与 URL 测速。
- `tls: true` 仅作用于 Clash API 路径；gRPC 路径使用不安全凭据。
- 本工具是管理客户端，不是流量中继。它不转发流量，也无法在 sing-box API
  不可达时工作。

## 开发

### 环境

```bash
nix develop   # 提供 go、buf、protoc-gen-go、golangci-lint、gopls
```

### 目标

```bash
make generate   # buf generate（重新生成 proto 绑定）
make build      # 构建 bin/sing-box-tui
make test       # go test ./...
make lint       # golangci-lint run ./...
```

### 目录结构

```
cmd/sing-box-tui   入口；参数解析、connect 子命令
internal/config    配置加载与校验
internal/client    API 客户端（Clash HTTP、gRPC）、会话状态
internal/tunnel    SSH 端口转发隧道
internal/app       Bubble Tea 模型、页面、快捷键
internal/ui        渲染辅助、样式
proto              gRPC API 服务的 protobuf 定义
gen                生成的 protobuf 代码
ref                sing-box-dashboard 参考仓库（submodule）
```

### 约定

- 构建相关工作统一通过 `make` 入口执行；工具链固定在 `flake.nix` 中，
  不临时拉取。
- `internal/client` 与 UI 解耦；所有 API 交互经由 `Session` 完成。
- 测试与被测代码同目录（`*_test.go`）；提交前先运行 `make test`。

## 许可证

[WTFPL —— Do What The Fuck You Want To Public License, Version 2](LICENSE)。