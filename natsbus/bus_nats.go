package natsbus

// 抽象出适配器方式， 所以推荐使用NewBus方式

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/suisrc/gobus"
)

// NatsBus - box for handlers and callbacks.
type NatsBus struct {
	nc       *nats.Conn
	handlers map[string][]*busHandler
	lock     sync.RWMutex // a lock for the map
}

type busHandler struct {
	topic    string
	group    string
	forward  string
	async    bool
	flagOnce bool
	call     reflect.Value
	sub      *nats.Subscription
}

var _ gobus.Bus = (*NatsBus)(nil)

// New returns new QueueBus with empty handlers.
func NewNatsBus(nc *nats.Conn) gobus.Bus {
	b := &NatsBus{
		nc,
		make(map[string][]*busHandler),
		sync.RWMutex{},
	}
	return gobus.Bus(b)
}

//=================================================================================================
//=================================================================================================
//=================================================================================================

// Unsubscribe removes callback defined for a topic.
// Returns error if there are no callbacks subscribed to the topic.
func (bus *NatsBus) Unsubscribe(topic string, handler interface{}) error {
	return bus.unsubscribe(topic, reflect.ValueOf(handler))
}

//=================================================================================================

func (bus *NatsBus) unsubscribe(topic string, hdl reflect.Value) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	if _, ok := bus.handlers[topic]; ok && len(bus.handlers[topic]) > 0 {
		if o := bus.removeHandler(topic, bus.findHandlerIdx(topic, hdl)); o != nil && o.sub != nil && o.sub.IsValid() {
			o.sub.Unsubscribe() // 订阅当前有效， 取消订阅
		}
		return nil
	}
	return fmt.Errorf("topic %s doesn't exist", topic)
}

func (bus *NatsBus) removeHandler(topic string, idx int) *busHandler {
	if _, ok := bus.handlers[topic]; !ok {
		return nil
	}
	l := len(bus.handlers[topic])

	if !(0 <= idx && idx < l) {
		return nil
	}
	o := bus.handlers[topic][idx]
	copy(bus.handlers[topic][idx:], bus.handlers[topic][idx+1:])
	bus.handlers[topic][l-1] = nil // or the zero value of T
	bus.handlers[topic] = bus.handlers[topic][:l-1]
	return o
}

func (bus *NatsBus) findHandlerIdx(topic string, callback reflect.Value) int {
	if _, ok := bus.handlers[topic]; ok {
		for idx, handler := range bus.handlers[topic] {
			if handler.call.Type() == callback.Type() &&
				handler.call.Pointer() == callback.Pointer() {
				return idx
			}
		}
	}
	return -1
}

//=================================================================================================
//=================================================================================================
//=================================================================================================

// func (?) (result, (error))， 注意订阅中包含 ">>" 是异步订阅
func (bus *NatsBus) Subscribe(topic string, fn interface{}) error {
	if topic, group, forward, async, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&busHandler{
		topic:   topic,
		group:   group,
		forward: forward,
		async:   async,
		call:    reflect.ValueOf(fn),
	}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))， 异步订阅忽略 ">>"，即无论任何情况下都是异步订阅
func (bus *NatsBus) SubscribeAsync(topic string, fn interface{}) error {
	if topic, group, forward, _, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&busHandler{
		topic:   topic,
		group:   group,
		forward: forward,
		async:   true,
		call:    reflect.ValueOf(fn),
	}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))
func (bus *NatsBus) SubscribeOnce(topic string, fn interface{}) error {
	if topic, group, forward, async, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&busHandler{
		topic:    topic,
		group:    group,
		forward:  forward,
		async:    async,
		call:     reflect.ValueOf(fn),
		flagOnce: true,
	}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))
func (bus *NatsBus) SubscribeOnceAsync(topic string, fn interface{}) error {
	if topic, group, forward, _, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&busHandler{
		topic:    topic,
		group:    group,
		forward:  forward,
		async:    true,
		call:     reflect.ValueOf(fn),
		flagOnce: true,
	}); err != nil {
		return err
	}
	return nil
}

//=================================================================================================

// topicA/groupA>>topicB
// 1.没有返回值的是异步订阅
// 2.订阅主题中包含 ">>" 是异步订阅
// 3.有返回值且主题中没有 ">>" 是同步订阅
func (bus *NatsBus) parseTopic(topic0 string, fn interface{}) (topic, group, forward string, async bool, err error) {
	topic0, err = gobus.GetTopicName(topic0)
	if err != nil {
		return // 配置无订阅， 跳过
	}
	if topic0 == "" {
		err = fmt.Errorf("[%v]订阅主题为空", fn)
		return // 无法订阅， 主题为空
	}
	topics := strings.SplitN(topic0, ">>", 2)
	if len(topics) == 2 {
		async = true // 包含">>"值, 异步订阅
		forward = strings.TrimSpace(topics[1])
	}
	topic2 := strings.SplitN(topics[0], "/", 2)
	if len(topic2) == 2 {
		group = strings.TrimSpace(topic2[1])
	}
	topic = strings.TrimSpace(topic2[0])
	if !async && reflect.ValueOf(fn).Type().NumOut() == 0 {
		async = true // 没有返回值， 异步订阅
	}
	return
}

// doSubscribe handles the subscription logic and is utilized by the public Subscribe functions
func (bus *NatsBus) doSubscribe(hdl *busHandler) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	if !(reflect.TypeOf(hdl.call.Interface()).Kind() == reflect.Func) {
		return fmt.Errorf("%s is not of type reflect.Func", reflect.TypeOf(hdl.call.Interface()).Kind())
	}
	subfunc := func(cb nats.MsgHandler) (*nats.Subscription, error) {
		if hdl.group == "" {
			return bus.nc.Subscribe(hdl.topic, cb)
		} else {
			return bus.nc.QueueSubscribe(hdl.topic, hdl.group, cb) // 分组
		}
	}
	if sub, err := subfunc(func(msg *nats.Msg) {
		bus.doHook(hdl, msg)
	}); err != nil {
		return err
	} else {
		hdl.sub = sub
	}
	bus.handlers[hdl.topic] = append(bus.handlers[hdl.topic], hdl)
	return nil
}

func (bus *NatsBus) doHook(hdl *busHandler, msg *nats.Msg) {
	tt := hdl.call.Type()
	cn := tt.NumIn()
	in := []reflect.Value{}
	if cn == 0 {
		// do nothing
	} else if cn == 1 {
		if v, err := FmtByte2RefVal(tt.In(0), msg.Data); err != nil {
			return // 无法处理内容
		} else {
			in = append(in, v)
		}
	} else {
		return // do nothing
	}
	// 执行调用, 处理结果
	if refv := hdl.call.Call(in); !hdl.async && msg.Reply != "" && len(refv) > 0 {
		bus.doReply(refv, msg.Reply)
	} else if hdl.async && hdl.forward != "" && len(refv) > 0 {
		bus.doReply(refv, hdl.forward)
	}
	if hdl.flagOnce { // 注销订阅
		bus.unsubscribe(hdl.topic, hdl.call)
	}
}

func (bus *NatsBus) doReply(refv []reflect.Value, topic string) {
	var value interface{}
	reply := false
	// func (?) (result, error) error优先返回
	if len(refv) > 1 && refv[1].Interface() != nil {
		value, reply = refv[1].Interface(), true
	} else if len(refv) > 0 && refv[0].Interface() != nil {
		value, reply = refv[0].Interface(), true
	}
	if reply { // 有结果需要返回
		if data, err := FmtData2Byte(value); err == nil {
			bus.nc.Publish(topic, data) // 返回数据
		}
	}
}

//=================================================================================================
//=================================================================================================
//=================================================================================================

// Publish executes callback defined for a topic. Any additional argument will be transferred to the callback.
func (bus *NatsBus) Publish(topic string, args interface{}) error {
	if bus.nc == nil && bus.nc.Status() != nats.CONNECTED { // 检查连接
		return fmt.Errorf("nats connection is nil or in invalid state")
	} else if data, err := FmtData2Byte(args); err != nil {
		return err // 数据无法序列化
	} else {
		return bus.nc.Publish(topic, data)
	}
}

// Publish executes callback defined for a topic. Any additional argument will be transferred to the callback.
func (bus *NatsBus) Request(topic string, timeout time.Duration, args interface{}, result interface{}) error {
	if bus.nc == nil || bus.nc.Status() != nats.CONNECTED { // 检查连接
		return fmt.Errorf("nats connection is nil or in invalid state")
	} else if data, err := FmtData2Byte(args); err != nil {
		return err // 数据无法序列化
	} else if msg, err := bus.nc.Request(topic, data, timeout); err != nil {
		return err // 推送内容发生异常
	} else if err := json.Unmarshal(msg.Data, result); err != nil {
		return err // 返回值无法反序列化
	}
	return nil
}

func (bus *NatsBus) RequestB(topic string, timeout time.Duration, args interface{}) ([]byte, error) {
	if bus.nc == nil && bus.nc.Status() != nats.CONNECTED { // 检查连接
		return nil, fmt.Errorf("nats connection is nil or in invalid state")
	} else if data, err := FmtData2Byte(args); err != nil {
		return nil, err
	} else if msg, err := bus.nc.Request(topic, data, timeout); err != nil {
		return nil, err
	} else {
		return msg.Data, nil
	}
}

//=================================================================================================

func FmtData2Byte(args interface{}) ([]byte, error) {
	switch args := args.(type) {
	case []byte:
		return args, nil
	case string:
		return []byte(args), nil
	default:
		return json.Marshal(args)
	}
}

func FmtByte2RefVal(t reflect.Type, data []byte) (reflect.Value, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.String() {
	case "[]uint8": // []byte
		return reflect.ValueOf(data), nil
	case "string":
		return reflect.ValueOf(string(data)), nil
	default:
		s := reflect.New(t) // 对象直接使用json格式化
		if err := json.Unmarshal(data, s.Interface()); err != nil {
			return reflect.ValueOf(nil), err // 无法处理
		}
		return s, nil
	}

}
