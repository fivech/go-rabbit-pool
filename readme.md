# go-rabbit-pool

> 引用包：[github.com/streadway/amqp](https://github.com/streadway/amqp)

## 功能实现
- 实现了多连接多信道的连接池。看了很多其他包，只实现了单连接的多信道连接池，这种方式在高并发时，单链接就不够用了
- 实现了连接过期时间设置。ps：连接的最后一个信道释放/关闭时，对应的连接如果过期，才进行关闭
- 日志打印，可以利用具体框架实现的日志模块打印，默认是标准输出。

## 使用教程
```go
package main

import (
	"fmt"
	"github.com/fivech/go-rabbit-pool"
	"github.com/streadway/amqp"
	"time"
)

func main() {
	var (
		pool *rabbit.Pool
		conf = rabbit.Config{
			Host:              "amqp://guest:guest@127.0.0.1:5672/diy-vhost", // rabbit地址
			MinConn:           1,                                             // 最少连接数量
			MaxConn:           10,                                            // 最大连接数量
			MaxChannelPerConn: 10,                                            // 每个连接的最大信道数量
			MaxLifetime:       time.Duration(3600),                           // 过期时间
		}
		err error
	)

	pool, err = rabbit.NewPool(&conf) // 建立连接池
	if err != nil {
		fmt.Println(err)
		return
	}
	ch, err := pool.GetChannel()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer ch.Release() // 释放信道
	err = ch.Publish("diy-exchange", "diy-routing-key", false, false, amqp.Publishing{
		ContentType:  "text/plain",
		Body:         []byte("111"),
		DeliveryMode: amqp.Transient,
		Priority:     0,
	})
	if err != nil {
		fmt.Println(err.Error())
	}
}

```

ps：更多使用，查看[pool_test.go](./pool_test.go)文件