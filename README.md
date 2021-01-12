# go cron server

结合go-cron-client使用

### dev

```
go run main.go -c dev.yml
```

### build

linux

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cron_server
```

mac

```
go build -o cron_server
```

run

```
./cron_server -c example.yml
```

### others

* 支持但不鼓励分钟以下定时任务。这种情形比较少见，在脚本中直接处理更加方便，也能与crontab保持兼容。
* 支持当定时任务上次没有结束时不再启动新进程，适用于大多数情况。默认不可修改。
* 不再支持多进程执行。这种情形比较少见，在脚本中直接处理更加方便。
* 暂时不支持暂停/重启等操作。简单才是硬道理。
* 支持但不鼓励跨服务器执行。
* 支持执行目录的配置。
* 暂时使用文件进行管理。好处是存储安全，简单，通过版本控制能保存历史，也有审核过程。缺点是在一些环境中上线复杂。
* 配置生效时间为一分钟，可修改代码，未提供配置。
* 简单的集群管理

### todo

* 管理
* 监控

* 为什么没有使用服务注册发现工具？ 为了保持简单性，现阶段暂不支持。
