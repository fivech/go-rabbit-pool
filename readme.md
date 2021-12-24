# go-rabbit-pool

> 引用包：[github.com/streadway/amqp](https://github.com/streadway/amqp)

## 功能实现
- 实现了多连接多信道的连接池。看了很多其他包，只实现了单连接的多信道连接池，这种方式在高并发时，单链接就不够用了
- 实现了连接过期时间设置。ps：连接的最后一个信道释放/关闭时，对应的连接如果过期，才进行关闭
- 日志打印，可以利用具体框架实现的日志模块打印，默认是标准输出。

## 使用教程
```go

```

ps：更多使用，查看[pool_test.go](./pool_test.go)文件