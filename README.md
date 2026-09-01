# hexas-config

hexas-config 是 lemongoff Go 服务使用的进程配置库。它只保留四段明确的生命周期：`Layout` 描述文件，`Loader` 构建不可变快照，`Snapshot` 提供只读访问，`Manager` 校验并原子发布快照。

最低 Go 版本为 1.24.12，推荐使用 Go 1.26.7 工具链。

## 设计

```text
Layout ──▶ Loader ──▶ Snapshot ──▶ Manager ──▶ Application
              │           │           ├── atomic publish
              │           │           ├── memory overrides
              │           │           └── watchers
              │           └── values / sources / metadata / safe dump
              └── template / overlay / placeholders / runtime overrides
```

- 没有包级默认实例或可变全局配置；每个 `Manager` 完全隔离。
- `Snapshot` 没有公开写方法，集合和元数据均以副本返回。
- `Loader` 不解析命令行。调用方完成 CLI 解析后，通过 `WithOverrides` 显式注入运行时值。
- 文件读取、占位符解析、覆盖、类型解码和业务校验全部成功后，`Manager` 才发布新快照。
- Watcher 回调在锁外同步执行，可以取消订阅或触发下一次重载。

## 代码结构

```text
hexas-config/
├── layout.go             文件位置与目录约定
├── loader.go             Loader 构造、显式覆盖和业务校验
├── pipeline.go           文件合并、占位符和环境覆盖流水线
├── snapshot.go           不可变配置快照与类型读取
├── metadata.go           结构化加载元数据
├── manager.go            校验、发布、内存覆盖和订阅生命周期
├── watcher.go            Watcher 公共类型
├── dump.go               排序且脱敏的诊断输出
└── internal/
    ├── resolver/         环境、文件和动态占位符来源
    └── watch/            并发安全的订阅注册表
```

根目录是唯一公开包。`internal` 仅包含不可替换的实现细节，避免把加载流水线拆成调用方无法正确组合的零散 API。

## 安装

```bash
go get github.com/lemongoff/hexas-config
```

## 配置目录

`DirectoryLayout("config/dev")` 使用以下约定：

```text
config/
  base/
    template.yaml          必需，完整结构与安全默认值
    global.yaml            可选，共享的非敏感占位符值
  dev/
    overlay.yaml           可选，环境差异
    replace.yaml           可选，当前环境的非敏感占位符值
    .env                   可选，仅限本地开发
```

`Layout` 也可以显式指定任意路径：

```go
layout := config.Layout{
    TemplateFile: "/etc/game/template.yaml",
    OverlayFile:  "/etc/game/production.yaml",
    ReplacementFiles: []config.ReplacementFile{
        {Path: "/etc/game/values.yaml", Type: "yaml"},
    },
}
```

`ReplacementFiles` 按声明顺序查找，先出现的值优先。进程环境变量高于所有占位符文件。

## 加载优先级

最终配置从高到低：

1. `Manager` 内存覆盖；
2. `WithOverrides` 显式运行时覆盖；
3. 与配置路径对应的进程环境变量；
4. `overlay.yaml`；
5. `template.yaml`。

配置键 `database.pool.max_open` 对应环境变量 `DATABASE_POOL_MAX_OPEN`。显式覆盖只作用于模板中已经声明的叶子键，不会绕过静态配置结构。

占位符从高到低查找：进程环境变量、`.env`、`replace.yaml`、`global.yaml`、`DYN_*` 动态值。缺失值和循环引用会使整次加载失败。

## 使用

### 加载快照

```go
loader, err := config.NewLoader(
    config.DirectoryLayout("config/prod"),
    config.WithOverrides("command-line", map[string]any{
        "server.port": 9090,
    }),
)
if err != nil {
    return err
}

snapshot, err := loader.Load()
if err != nil {
    return err
}

port := snapshot.GetInt("server.port")
source := snapshot.Source("server.port")
```

### 解码和校验

```go
type AppConfig struct {
    Server struct {
        Port int `mapstructure:"port"`
    } `mapstructure:"server"`
}

func (config *AppConfig) Validate() error {
    if config.Server.Port <= 0 {
        return errors.New("server.port must be positive")
    }
    return nil
}

var appConfig AppConfig
snapshot, err := loader.LoadInto(&appConfig)
```

实现 `Validate() error` 后，跨字段约束会在快照返回或发布前执行。

### 原子发布与重载

```go
manager, err := config.NewManager(loader)
if err != nil {
    return err
}

var appConfig AppConfig
if err := manager.ReloadInto(&appConfig); err != nil {
    return err
}

current := manager.Current()
```

加载或校验失败时，`Current()` 继续返回上一个有效快照。第一次成功发布前返回 `nil`。

### 临时内存覆盖

```go
if err := manager.SetMemory("feature.enabled", true, "operations"); err != nil {
    return err
}
snapshot, err := manager.ReloadMemory()
```

内存值只存在于当前进程。`ClearMemory` 清空它们；正式文件 `Reload` 成功后也会自动清空，避免临时值成为事实配置源。

### 监听

```go
cancel := manager.Subscribe([]string{"feature.enabled"}, func(key, value string) {
    // Keep callbacks short; they run synchronously after publication.
})
defer cancel()
```

键名会转换为小写并去除首尾空白，重复键只注册一次。注册表不会在持锁时调用回调。

### 安全诊断

```go
var output bytes.Buffer
err := manager.Current().DumpTo(
    &output,
    config.WithSensitiveKeys("payment.signing_material"),
)
```

名称包含 `secret`、`password`、`passwd`、`token`、`credential`、`privatekey`、`apikey` 或 `dsn` 的键默认输出 `[REDACTED]`。`Settings()` 返回原始值，只能用于受控的进程内逻辑，不应直接写入日志。

## API

| 类型 | 责任 |
| --- | --- |
| `Layout` / `ReplacementFile` | 显式描述输入文件 |
| `Loader` | 构建隔离快照，可选解码和校验 |
| `WithOverrides` | 注入调用方已经解析的运行时值 |
| `Snapshot` | 类型读取、来源查询、元数据和脱敏输出 |
| `Metadata` | 描述本次加载使用的文件与来源 |
| `Manager` | 原子发布、失败保留、内存覆盖和订阅 |
| `Watcher` | 接收已发布快照中的指定键值 |

## 动态变量

| 占位符 | 含义 |
| --- | --- |
| `${DYN_LOCAL_HOST}` | 当前主机名 |
| `${DYN_USER_NAME}` | 当前系统用户名 |
| `${DYN_WORKSPACE_ROOT}` | 当前工作目录 |
| `${DYN_WORKSPACE_ROOT.delspace}` | 删除空白后的当前工作目录 |

动态值只能提供非敏感运行信息，不能生成身份凭证或授权结论。

## 边界

- 本库不监听文件系统；调用方应在受控信号下执行 `Reload`。
- 本库不拉取远端秘密或远端配置。
- 本库不负责 project client 游戏配置的草稿、发布、灰度、revision 或回滚。
- Watcher 是进程内同步通知，不提供跨进程事件或持久化审计。

## 开发与验证

```bash
go fmt ./...
go vet ./...
go test -race ./...
git diff --check
```

贡献前请阅读 [AI 协作与工程约束](AGENTS.md)。
