package natsbus

import (
	"encoding/json"
	"fmt"
	"reflect"
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
	call     reflect.Value
	sub      *nats.Subscription
	topic    string
	group    string
	async    bool
	flagOnce bool
}

var _ gobus.Bus = (*NatsBus)(nil)

// New returns new QueueBus with empty handlers.
func New(nc *nats.Conn) gobus.Bus {
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
		var value interface{}
		reply := false
		// func (?) (result, error)
		if len(refv) > 1 && refv[1].Interface() != nil {
			value, reply = refv[1].Interface(), true
		} else if len(refv) > 0 && refv[0].Interface() != nil {
			value, reply = refv[0].Interface(), true
		}
		if reply { // 有结果需要返回
			if data, err := FmtData2Byte(value); err == nil {
				bus.nc.Publish(msg.Reply, data) // 返回数据
			}
		}
	}
	if hdl.flagOnce { // 注销订阅
		bus.unsubscribe(hdl.topic, hdl.call)
	}
}

//=================================================================================================

// func (?) (result, (error))
func (bus *NatsBus) Subscribe(topic string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn)}); err != nil {
		return err
	}
	return nil
}

// func (?)
func (bus *NatsBus) SubscribeAsync(topic string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn), async: true}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))
func (bus *NatsBus) SubscribeOnce(topic string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn), flagOnce: true}); err != nil {
		return err
	}
	return nil
}

// func (?)
func (bus *NatsBus) SubscribeOnceAsync(topic string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn), async: true, flagOnce: true}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))
func (bus *NatsBus) SubscribeByGroup(topic, group string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn), group: group}); err != nil {
		return err
	}
	return nil
}

// func (?)
func (bus *NatsBus) SubscribeAsyncByGroup(topic, group string, fn interface{}) error {
	if err := bus.doSubscribe(&busHandler{topic: topic, call: reflect.ValueOf(fn), group: group, async: true}); err != nil {
		return err
	}
	return nil
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
