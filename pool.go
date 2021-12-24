package rabbit

import (
	"errors"
	"github.com/streadway/amqp"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultLogPrefix = "[rabbit-pool]"
)

var (
	ErrInvalidConfig     = errors.New("invalid pool config\n")
	ErrFailedConnection  = errors.New("failed to establish connection\n")
	ErrConnectionMaximum = errors.New("the number of connections exceeds the maximum\n")
	ErrChannelMaximum    = errors.New("the number of channels exceeds the maximum\n")
	ErrGetChannelTimeOut = errors.New("get channel timeout\n")
)

type LoggerInter interface {
	Print(v ...interface{})
}

type Config struct {
	Host              string // MQ的地址
	MinConn           int    // 最少建立的连接数
	MaxConn           int    // 最大建立的连接数
	MaxChannelPerConn int    // 每个连接最多建立的信道数量
	MaxLifetime       time.Duration
}

// 连接池
type Pool struct {
	mu                     *sync.Mutex
	conf                   *Config
	logger                 LoggerInter
	connectionNum          int32
	connections            map[int64]*Connection
	connectionSerialNumber int64
	idleChannels           chan *Channel
}

func NewPool(conf *Config, logger ...LoggerInter) (*Pool, error) {
	if conf.MaxConn <= 0 || conf.MinConn > conf.MaxConn {
		return nil, ErrInvalidConfig
	}
	p := &Pool{
		mu:           new(sync.Mutex),
		connections:  make(map[int64]*Connection),
		idleChannels: make(chan *Channel, conf.MaxConn*conf.MaxChannelPerConn),
	}

	if conf.MaxLifetime == 0 {
		conf.MaxLifetime = time.Duration(3600)
	}

	if len(logger) > 0 {
		p.SetLogger(logger[0])
	} else {
		p.SetLogger(log.New(os.Stdout, defaultLogPrefix, log.LstdFlags))
	}
	p.conf = conf

	var conn *Connection
	var err error
	// 建立最少连接数
	for i := 0; i < conf.MinConn; i++ {
		conn, err = p.NewConnection()
		if err != nil {
			p.GetLogger().Print(ErrFailedConnection.Error())
			return nil, ErrFailedConnection
		}
		p.connections[conn.connIdentity] = conn
	}
	return p, nil
}

func (p *Pool) SetConfig(conf *Config) *Pool {
	p.conf = conf
	return p
}

func (p *Pool) GetConfig() *Config {
	return p.conf
}

func (p *Pool) SetLogger(logger LoggerInter) *Pool {
	p.logger = logger
	return p
}

func (p *Pool) GetLogger() LoggerInter {
	return p.logger
}

func (p *Pool) NewConnection() (*Connection, error) {
	// 判断连接是否达到最大值
	if atomic.AddInt32(&p.connectionNum, 1) > int32(p.conf.MaxConn) {
		atomic.AddInt32(&p.connectionNum, -1)
		return nil, ErrConnectionMaximum
	}
	conn, err := amqp.Dial(p.conf.Host)
	if err != nil {
		atomic.AddInt32(&p.connectionNum, -1)
		return nil, err
	}

	return &Connection{
		mu:           new(sync.Mutex),
		conn:         conn,
		pool:         p,
		channelNum:   0,
		expireTime:   time.Duration(time.Now().Unix()) + p.conf.MaxLifetime,
		connIdentity: atomic.AddInt64(&p.connectionSerialNumber, 1),
	}, nil
}

func (p *Pool) CloseConnection(c *Connection) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	atomic.AddInt32(&p.connectionNum, -1)
	delete(p.connections, c.connIdentity)
	return c.conn.Close()
}

func (p *Pool) GetChannel() (*Channel, error) {
	ch, _ := p.getOrCreate()
	if ch != nil {
		return ch, nil
	}

	C := time.After(time.Second * 10)
	for {
		ch, _ := p.getOrCreate()
		if ch != nil {
			return ch, nil
		}
		select {
		case <-C:
			p.GetLogger().Print(ErrGetChannelTimeOut.Error())
			return nil, ErrGetChannelTimeOut
		default:
		}
	}
}

func (p *Pool) getOrCreate() (*Channel, error) {
	// 池中是否有空闲channel
	var (
		ch  *Channel
		err error
	)
	select {
	case ch = <-p.idleChannels:
		return ch, nil
	default:
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// 池中已有连接是否可以建立新的channel
	for _, conn := range p.connections {
		if conn.CheckExpire() {
			continue
		}
		ch, err = conn.NewChannel()
		if ch != nil {
			return ch, nil
		}
	}
	// 新建连接获取新的channel
	var conn *Connection
	conn, err = p.NewConnection()
	if err != nil {
		return nil, err
	}
	p.connections[conn.connIdentity] = conn
	ch, err = conn.NewChannel()
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (p *Pool) ReleaseChannel(ch *Channel) error {
	p.idleChannels <- ch
	return nil
}

type Connection struct {
	mu                  *sync.Mutex
	conn                *amqp.Connection
	pool                *Pool
	expireTime          time.Duration
	isExpire            bool
	connIdentity        int64 // 连接标记
	channelNum          int32 // 该连接的信道数量
	channelSerialNumber int64 // 第几个channel
}

func (c *Connection) NewChannel() (*Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if atomic.AddInt32(&c.channelNum, 1) > int32(c.pool.conf.MaxChannelPerConn) {
		atomic.AddInt32(&c.channelNum, -1)
		return nil, ErrChannelMaximum
	}
	ch, err := c.conn.Channel()
	if err != nil {
		atomic.AddInt32(&c.channelNum, -1)
		return nil, err
	}
	return &Channel{
		Channel:      ch,
		conn:         c,
		chanIdentity: atomic.AddInt64(&c.channelSerialNumber, 1),
	}, nil
}

func (c *Connection) ReleaseChannel(ch *Channel) error {
	if c.CheckExpire() {
		return c.CloseChannel(ch)
	}
	return c.pool.ReleaseChannel(ch)
}

func (c *Connection) CloseChannel(ch *Channel) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	atomic.AddInt32(&c.channelNum, -1)
	var err = ch.Channel.Close()
	if atomic.LoadInt32(&c.channelNum) <= 0 && c.CheckExpire() {
		return c.pool.CloseConnection(c)
	}
	return err
}

// 检查是否过期
func (c *Connection) CheckExpire() bool {
	if c.isExpire {
		return true
	}
	if time.Duration(time.Now().Unix()) > c.expireTime {
		c.isExpire = true
	}
	return c.isExpire
}

type Channel struct {
	*amqp.Channel
	conn         *Connection
	chanIdentity int64 // 该连接的第几个channel
}

func (ch *Channel) Release() error {
	return ch.conn.ReleaseChannel(ch)
}

func (ch *Channel) Close() error {
	return ch.conn.CloseChannel(ch)
}
