package gobus

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// BusProvider - box for handlers and callbacks.
type BusProvider struct {
	adapter  BusAdapter //
	handlers map[string][]*BusHandler
	lock     sync.RWMutex // a lock for the map
}

type BusAdapter interface {
	Name() string
	Conn() interface{}
	Valid() error
	Subscribe(topic, group string, fn func(Message)) (sub interface{}, err error)
	Unsubscribe(sub interface{}) error
	Publish(topic string, data []byte) error
	Request(topic string, data []byte, timeout time.Duration) (Message, error)
}

type BusHandler struct {
	topic    string
	group    string
	forward  string
	async    bool
	flagOnce bool
	call     reflect.Value
	sub      interface{}
}

type Message interface {
	GetSubject() string //
	GetReply() string   //
	GetData() []byte    //
}

var _ Bus = (*BusProvider)(nil)

// New returns new QueueBus with empty handlers.
func NewProvider(adapter BusAdapter) *BusProvider {
	return &BusProvider{
		handlers: make(map[string][]*BusHandler),
		lock:     sync.RWMutex{},
		adapter:  adapter,
	}
}

func (bus *BusProvider) SetApater(adapter BusAdapter) {
	bus.adapter = adapter
}

//=================================================================================================
//=================================================================================================
//=================================================================================================

// Unsubscribe removes callback defined for a topic.
// Returns error if there are no callbacks subscribed to the topic.
func (bus *BusProvider) Unsubscribe(topic string, handler interface{}) error {
	return bus.unsubscribe(topic, reflect.ValueOf(handler))
}

//=================================================================================================

func (bus *BusProvider) unsubscribe(topic string, hdl reflect.Value) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	if _, ok := bus.handlers[topic]; ok && len(bus.handlers[topic]) > 0 {
		if o := bus.removeHandler(topic, bus.findHandlerIdx(topic, hdl)); o != nil {
			bus.adapter.Unsubscribe(o) // 订阅当前有效， 取消订阅
		}
		return nil
	}
	return fmt.Errorf("topic %s doesn't exist", topic)
}

func (bus *BusProvider) removeHandler(topic string, idx int) *BusHandler {
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

func (bus *BusProvider) findHandlerIdx(topic string, callback reflect.Value) int {
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
func (bus *BusProvider) Subscribe(topic string, fn interface{}) error {
	if topic, group, forward, async, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&BusHandler{
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
func (bus *BusProvider) SubscribeAsync(topic string, fn interface{}) error {
	if topic, group, forward, _, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&BusHandler{
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
func (bus *BusProvider) SubscribeOnce(topic string, fn interface{}) error {
	if topic, group, forward, async, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&BusHandler{
		topic:    topic,
		group:    group,
		forward:  forward,
		async:    async,
		flagOnce: true,
		call:     reflect.ValueOf(fn),
	}); err != nil {
		return err
	}
	return nil
}

// func (?) (result, (error))
func (bus *BusProvider) SubscribeOnceAsync(topic string, fn interface{}) error {
	if topic, group, forward, _, err := bus.parseTopic(topic, fn); err != nil {
		return err
	} else if err := bus.doSubscribe(&BusHandler{
		topic:    topic,
		group:    group,
		forward:  forward,
		async:    true,
		flagOnce: true,
		call:     reflect.ValueOf(fn),
	}); err != nil {
		return err
	}
	return nil
}

//=================================================================================================

// topicA#groupA>>topicB
// 1.没有返回值的是异步订阅
// 2.订阅主题中包含 ">>" 是异步订阅
// 3.有返回值且主题中没有 ">>" 是同步订阅
func (bus *BusProvider) parseTopic(topic0 string, fn interface{}) (topic, group, forward string, async bool, err error) {
	topic0, err = GetTopicName(topic0)
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
	topic2 := strings.SplitN(topics[0], "#", 2)
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
func (bus *BusProvider) doSubscribe(hdl *BusHandler) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	if !(reflect.TypeOf(hdl.call.Interface()).Kind() == reflect.Func) {
		return fmt.Errorf("%s is not of type reflect.Func", reflect.TypeOf(hdl.call.Interface()).Kind())
	}
	if sub, err := bus.adapter.Subscribe(hdl.topic, hdl.group, func(msg Message) {
		bus.doHook(hdl, msg)
	}); err != nil {
		return err
	} else {
		hdl.sub = sub
	}
	bus.handlers[hdl.topic] = append(bus.handlers[hdl.topic], hdl)
	return nil
}

func (bus *BusProvider) doHook(hdl *BusHandler, msg Message) {
	tt := hdl.call.Type()
	cn := tt.NumIn()
	in := []reflect.Value{}
	if cn == 0 {
		// do nothing
	} else if cn == 1 {
		if v, err := FmtByte2RefVal(tt.In(0), msg.GetData()); err != nil {
			return // 无法处理内容
		} else {
			in = append(in, v)
		}
	} else {
		return // do nothing
	}
	// 执行调用, 处理结果
	if refv := hdl.call.Call(in); !hdl.async && msg.GetReply() != "" && len(refv) > 0 {
		bus.doReply(refv, msg.GetReply())
	} else if hdl.async && hdl.forward != "" && len(refv) > 0 {
		bus.doReply(refv, hdl.forward)
	}
	if hdl.flagOnce { // 注销订阅
		bus.unsubscribe(hdl.topic, hdl.call)
	}
}

func (bus *BusProvider) doReply(refv []reflect.Value, topic string) {
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
			bus.adapter.Publish(topic, data) // 返回数据
		}
	}
}

//=================================================================================================
//=================================================================================================
//=================================================================================================

// Publish executes callback defined for a topic. Any additional argument will be transferred to the callback.
func (bus *BusProvider) Publish(topic string, args interface{}) error {
	if err := bus.adapter.Valid(); err != nil { // 检查连接
		return err
	} else if data, err := FmtData2Byte(args); err != nil {
		return err // 数据无法序列化
	} else {
		return bus.adapter.Publish(topic, data)
	}
}

// Publish executes callback defined for a topic. Any additional argument will be transferred to the callback.
func (bus *BusProvider) Request(topic string, timeout time.Duration, args interface{}, result interface{}) error {
	if err := bus.adapter.Valid(); err != nil { // 检查连接
		return err
	} else if data, err := FmtData2Byte(args); err != nil {
		return err // 数据无法序列化
	} else if msg, err := bus.adapter.Request(topic, data, timeout); err != nil {
		return err // 推送内容发生异常
	} else if err := json.Unmarshal(msg.GetData(), result); err != nil {
		return err // 返回值无法反序列化
	}
	return nil
}

func (bus *BusProvider) RequestB(topic string, timeout time.Duration, args interface{}) ([]byte, error) {
	if err := bus.adapter.Valid(); err != nil { // 检查连接
		return nil, err
	} else if data, err := FmtData2Byte(args); err != nil {
		return nil, err
	} else if msg, err := bus.adapter.Request(topic, data, timeout); err != nil {
		return nil, err
	} else {
		return msg.GetData(), nil
	}
}

func (bus *BusProvider) RequestO(topic string, timeout time.Duration, args interface{}) (Message, error) {
	if err := bus.adapter.Valid(); err != nil { // 检查连接
		return nil, err
	} else if data, err := FmtData2Byte(args); err != nil {
		return nil, err
	} else if msg, err := bus.adapter.Request(topic, data, timeout); err != nil {
		return nil, err
	} else {
		return msg, nil
	}
}
