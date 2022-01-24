package natsbus

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/suisrc/gobus"
)

var _ gobus.BusAdapter = (*NatsAdapter)(nil)
var _ gobus.Message = (*Message)(nil)

// NatsAdapter - nats adapter
type NatsAdapter struct {
	conn *nats.Conn
}

// New returns new QueueBus with nats adapter.
func New(nc *nats.Conn) gobus.Bus {
	p := &NatsAdapter{nc}
	b := gobus.NewProvider(p)
	return gobus.Bus(b)
}

//=========================================================================================================

func (bus *NatsAdapter) Name() string {
	return "nats " + nats.Version
}

func (bus *NatsAdapter) Conn() interface{} {
	return bus.conn
}

func (bus *NatsAdapter) Valid() error {
	if bus.conn == nil && bus.conn.Status() != nats.CONNECTED {
		return fmt.Errorf("nats connection is nil or in invalid state")
	}
	return nil
}

func (bus *NatsAdapter) Subscribe(topic, group string, fn func(gobus.Message)) (sub interface{}, err error) {
	if group == "" { // 无队列订阅
		return bus.conn.Subscribe(topic, func(msg *nats.Msg) { fn(&Message{msg}) })
	} else { // 有队列订阅
		return bus.conn.QueueSubscribe(topic, group, func(msg *nats.Msg) { fn(&Message{msg}) }) // 队列订阅
	}
}

func (bus *NatsAdapter) Unsubscribe(sub interface{}) error {
	if err := bus.Valid(); err != nil {
		return err // 总线无效
	}
	if sub == nil {
		return nil
	}
	if sb, ok := sub.(*nats.Subscription); ok && sb.IsValid() {
		return sb.Unsubscribe() // 取消订阅
	}
	return nil
}

func (bus *NatsAdapter) Publish(topic string, data []byte) error {
	return bus.conn.Publish(topic, data) // 发布消息
}

func (bus *NatsAdapter) Request(topic string, data []byte, timeout time.Duration) (gobus.Message, error) {
	if msg, err := bus.conn.Request(topic, data, timeout); err != nil { // 请求消息
		return nil, err
	} else {
		return &Message{msg}, nil
	}
}

//=========================================================================================================

type Message struct {
	*nats.Msg
}

func (msg *Message) GetSubject() string {
	return msg.Msg.Subject
}

func (msg *Message) GetReply() string {
	return msg.Msg.Reply
}

func (msg *Message) GetData() []byte {
	return msg.Msg.Data
}
