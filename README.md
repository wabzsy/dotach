# 简介

hvv红队渗透测试工具, 劫持进程输入输出的工具

# 使用场景

> 如: 现在拿到了一台linux服务器(A), ps 命令查看进程发现有用户ssh连接到了其他服务器(B,C,D...), 现在想要通过A现有的这个ssh连接来直接控制B,C,D等服务器

# 特点

- go语言编写, 方便快速的跨平台编译
- 不依赖目标机操作系统版本(基本上只要是基于linux的就行)
- 目前支持 amd64 / arm64 架构
- 直接劫持现有的ssh连接, 不需要等待新的ssh连接
- 劫持时原ssh客户端无感知, 退出劫持后原ssh客户端不会有任何异常显示(在你正确使用本工具的前提下)
- 不需要修改系统原有文件, 不会触发`文件被篡改`之类的报警
- 支持`root用户`劫持`root用户`的进程
- 支持`root用户`劫持`非root用户`的进程
- 支持`非root用户`劫持`非root用户`的进程
- 不支持`非root用户`劫持`root用户`的进程(想的咋那么美呢)
- 其实不光可以劫持ssh连接, 具体的你就自己探索吧..

# 编译方式

## Linux amd64

`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dotach`

## Linux arm64(aarch64)

`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dotach`

目前只支持这两种架构

# 使用方式

1. 查看查看进程中是否有ssh会话
2. 把目标进程PID记下来
3. `./dotach -p 目标进程的PID` 开始劫持
4. 使用`Ctrl+X Ctrl+X Ctrl+X`退出劫持状态

# 注意事项

- 目标进程不能处于被调试状态
- 具体使用细节请看源码
