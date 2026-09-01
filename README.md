# Hexas Config

`hexas-config` 是 Hexas 游戏服务的类型化进程配置库。它使用显式默认值、有序配置来源、严格 YAML 解码、业务校验和原子快照发布，不提供旧 go-zero 配置 API 的兼容层。

## 安装

```bash
go get github.com/lemongoff/hexas-config
```

正式发布前应固定明确 commit；不要使用漂移的 `@latest`。

## 配置领域

- Bootstrap Config：监听地址、数据库、Redis、日志、TLS 和服务发现等启动配置，通常来自文件、环境变量和命令行覆盖。
- Runtime Config：功能开关、活动参数、区服策略等运行时配置，可使用独立的 etcd Manager。远端 Runtime Config 不应覆盖 Bootstrap Config。

## 核心流程

```text
typed defaults
  -> ordered Source documents
  -> strict decode
  -> Validate
  -> immutable Snapshot[T]
  -> atomic publish
  -> Watcher notifications
```

任一步失败都会保留上一版有效快照，并且不会通知订阅者。

## 静态配置

```go
manager, err := config.NewManager(
    appconfig.Default(),
    config.YAMLFile("config/base.yaml"),
    config.OptionalYAMLFile("config/env/dev.yaml"),
    config.Environment("HEXAS_"),
)
if err != nil {
    return err
}
if err := manager.Load(ctx); err != nil {
    return err
}
snapshot, ok := manager.Current()
if !ok {
    return errors.New("configuration was not published")
}
current := snapshot.Value()
```

来源按声明顺序合并，后声明的来源优先。环境变量 `HEXAS_SERVER_PORT` 映射为 `server.port`。未知字段会导致加载失败。

## 默认值和校验

默认值由调用方提供，不使用反射标签：

```go
func Default() AppConfig {
    return AppConfig{Port: 8080}
}

func (c AppConfig) Validate() error {
    if c.Port <= 0 {
        return errors.New("port must be positive")
    }
    return nil
}
```

Manager 总是解码到新的配置实例，并在 `Validate` 成功后发布。

## 内存覆盖

```go
if err := manager.SetMemory("feature.enabled", true); err != nil {
    return err
}
if err := manager.Load(ctx); err != nil {
    return err
}
```

内存覆盖拥有最高优先级，但仍必须通过完整类型解码和业务校验。

## etcd Runtime Config

```go
runtimeSource, err := etcdconfig.New(etcdconfig.Config{
    Client: client,
    Key: "/hexas/prod/game/match/runtime",
    Timeout: 5 * time.Second,
})
manager, err := config.NewManager(DefaultRuntimeConfig(), runtimeSource)
```

`source/etcd` 只监听精确 key，调用方持有并关闭 etcd client。配置值是 YAML；revision 记录在 Snapshot metadata 中。

## 安全诊断

```go
err := snapshot.DumpTo(writer, config.WithSensitiveKeys("payment.signing_material"))
```

名称包含 `secret`、`password`、`passwd`、`token`、`credential`、`privatekey`、`apikey`、`dsn` 或 `pass` 的键默认输出 `[REDACTED]`。

## 明确不支持

- JSON5、TOML 和 properties 配置输入
- 包级全局配置实例
- 库内部 `log.Fatal`、`os.Exit` 或 panic
- 旧 `Layout`、`Loader`、字符串 Getter 和字符串 Watcher API
- 游戏策划配置的审批、灰度平台和持久化审计
- 把 Runtime Config 用作秘密存储

## 验证

```bash
go fmt ./...
go vet ./...
go test -race ./...
git diff --check
```
