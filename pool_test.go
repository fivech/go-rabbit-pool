package rabbit

import (
	"fmt"
	"github.com/streadway/amqp"
	"strconv"
	"testing"
	"time"
)

const (
	DiyExchange   = "diy-exchange"    // 自定义的交换器名称
	DiyQueue      = "diy-queue"       // 自定义的队列名称
	DiyRoutingKey = "diy-routing-key" // 自定义绑定的路由键
)

var (
	pool *Pool
	conf = Config{
		Host:              "amqp://guest:guest@127.0.0.1:5672/diy-vhost", // rabbit地址
		MinConn:           10,                                            // 最少连接数量
		MaxConn:           50,                                            // 最大连接数量
		MaxChannelPerConn: 10,                                            // 每个连接的最大信道数量
		MaxLifetime:       time.Duration(3600),                           // 过期时间
	}
)

func TestMain(m *testing.M) {
	var err error
	pool, err = NewPool(&conf) // 建立连接池
	if err != nil {
		fmt.Print(err.Error())
		return
	}

	fmt.Println("准备工作")
	ch, err := pool.GetChannel()
	if err != nil {
		fmt.Print(err.Error())
		return
	}

	if err = ch.Qos(1, 0, true); err != nil {
		fmt.Printf("failed to set qos %s", err.Error())
		return
	}

	err = ch.ExchangeDeclare(
		DiyExchange, // name
		"topic",     // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		fmt.Printf("failed to declare an exchange %s", err.Error())
		return
	}

	q, err := ch.QueueDeclare(
		DiyQueue, // name
		true,     // durable
		false,    // delete when unused
		false,    // exclusive 这个独家会影响durable
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		fmt.Printf("failed to declare a queue %s", err.Error())
		return
	}

	err = ch.QueueBind(
		q.Name,        // queue name
		DiyRoutingKey, // routing key
		DiyExchange,   // exchange
		false,
		nil)
	if err != nil {
		fmt.Printf("failed to bind a queue %s", err.Error())
		return
	}
	ch.Release()
	m.Run()
}

func TestPool_GetChannel(t *testing.T) {
	ch, err := pool.GetChannel()
	if err != nil {
		t.Error(err)
		return
	}
	defer ch.Release()
	err = ch.Publish(DiyExchange, DiyRoutingKey, false, false, amqp.Publishing{
		ContentType:  "text/plain",
		Body:         []byte("111"),
		DeliveryMode: amqp.Transient,
		Priority:     0,
	})
	if err != nil {
		t.Error(err)
	}
}

func Example_Consume() {
	ch, err := pool.GetChannel()
	if err != nil {
		fmt.Println(err.Error())
	}
	defer ch.Release()
	delivery, err := ch.Consume(DiyQueue, "", false, false, false, false, nil)
	if err != nil {
		fmt.Println(err)
	}
	var res amqp.Delivery
	var ok bool
	//for {
	res, ok = <-delivery
	if ok == true {
		fmt.Println(string(res.Body))
		_ = res.Ack(false)
	} else {
		fmt.Println("error getting message")
		//break
	}
	//}
	fmt.Println("get msg done")
	// Output:
	// 111
	// get msg done
}

func BenchmarkPool_GetChannel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		go func(i int) {
			ch, err := pool.GetChannel()
			if err != nil {
				fmt.Println(err)
			}
			err = ch.Publish(DiyExchange, DiyRoutingKey, false, false, amqp.Publishing{
				ContentType:  "text/plain",
				Body:         []byte(strconv.Itoa(i)),
				DeliveryMode: amqp.Transient,
				Priority:     0,
			})
			ch.Release()
			if err != nil {
				fmt.Println(err)
			}
		}(i)

	}
}
