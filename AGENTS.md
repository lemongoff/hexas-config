# hexas-config AI 协作与工程约束

本仓库提供 Hexas 的类型化进程配置基础设施，不兼容旧 go-zero `core/conf` 或旧版 hexas-config API。

## 设计边界

- 公开核心是 `Source`、`WatchSource`、`Manager[T]`、`Snapshot[T]`、`Change[T]`。
- 调用方显式提供类型默认值；本库不解释 `default`、`range`、`options` 反射标签。
- 配置输入统一为 YAML、Map、前缀环境变量或明确的远端 Source。
- Bootstrap Config 与 Runtime Config 使用不同 Manager，远端来源不得覆盖启动秘密和基础设施参数。
- etcd provider 直接依赖 etcd client，不依赖 Hexas 根 module。

## 安全与并发

- 新配置必须完成来源读取、严格解码和业务校验后才原子发布。
- 失败保留旧快照，不通知 Watcher。
- Snapshot 返回的值、集合和元数据必须与内部状态隔离。
- 内存覆盖也必须重新解码和校验。
- 不在错误、日志、测试数据或 metadata 中输出秘密。
- Watcher 在锁外运行；panic 必须隔离，慢回调应由调用方治理。
- 网络来源必须接收 context、使用正数 timeout，并明确 client 所有权。

## 工程规则

- Go 版本与 Hexas 保持一致。
- 错误使用 `%w` 保留根因；库代码不 panic、不退出进程。
- 公共 API、来源优先级、校验、revision 和重载语义变化必须同步 README 和测试。
- 未经用户明确要求，不执行 commit、tag 或 push。

## 验证

```bash
go fmt ./...
go vet ./...
go test -race ./...
git diff --check
```
