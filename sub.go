package gobus

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// BusKind 账户类型
type Kind int

const (
	BusSync      Kind = iota // value -> 0
	BusAsync                 // value -> 1
	BusOnceSync              // value -> 2, 不支持group
	BusOnceAsync             // value -> 3, 不支持group
)

type EventSubscriber interface {
	Subscribe(Bus) (func(), error)
}

//=======================================================================================

type EventHandler interface {
	Subscribe() (kind Kind, topic string, handler interface{})
}

/**
 * 订阅模式
 * bus: 总线
 * hdl: 订阅的内容
 */
func Subscribe(bus Bus, hdl EventHandler) (func(), error) {
	kind, topic, handler := hdl.Subscribe()
	switch kind {
	case BusAsync:
		if err := bus.SubscribeAsync(topic, handler); err != nil {
			return nil, err
		}
	case BusOnceSync:
		if err := bus.SubscribeOnce(topic, handler); err != nil {
			return nil, err
		}
	case BusOnceAsync:
		if err := bus.SubscribeOnceAsync(topic, handler); err != nil {
			return nil, err
		}
	default:
		if err := bus.Subscribe(topic, handler); err != nil {
			return nil, err
		}
	}
	return func() { bus.Unsubscribe(topic, handler) }, nil
}

//=======================================================================================

type EventHandlerByGroup interface {
	Subscribe() (kind Kind, topic, group string, handler interface{})
}

/**
 * 队列模式
 * bus: 总线
 * hdl: 订阅的内容
 */
func SubscribeByGroup(bus Bus, hdl EventHandlerByGroup) (func(), error) {
	kind, topic, group, handler := hdl.Subscribe()
	switch kind {
	case BusSync:
		if err := bus.SubscribeByGroup(topic, group, handler); err != nil {
			return nil, err
		}
	case BusAsync:
		if err := bus.SubscribeAsyncByGroup(topic, group, handler); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("group is no suppert type: %v", kind)
	}
	return func() { bus.Unsubscribe(topic, handler) }, nil
}

//=======================================================================================

/**
 *
 * bus: 总线
 * data: 订阅集合
 * verify: true: 发现错误，立即返回
 * excludes: 集合中排除的属性
 */
func SubscribeBatch(bus Bus, data interface{}, verify bool, excludes ...string) (func(), error) {
	sort.Strings(excludes)
	clss := []func(){}
	errs := []error{}
	typ := reflect.TypeOf(data).Elem() // 指针
	val := reflect.ValueOf(data).Elem()
	for k := 0; k < typ.NumField(); k++ {
		if idx := SearchStrings(&excludes, typ.Field(k).Name); idx >= 0 {
			continue
		}
		var cls func()
		var err error
		switch value := val.Field(k).Interface().(type) {
		case EventHandler:
			cls, err = Subscribe(bus, value)
		case EventHandlerByGroup:
			cls, err = SubscribeByGroup(bus, value)
		case EventSubscriber:
			cls, err = value.Subscribe(bus)
		}
		if err != nil && verify {
			return nil, err
		} else if err != nil {
			errs = append(errs, err)
		} else if cls != nil {
			clss = append(clss, cls)
		}
	}
	clear := func() {
		for _, opt := range clss {
			opt()
		}
	}
	if len(errs) > 0 {
		return clear, NewMultiError(&errs)
	}
	return clear, nil
}

func SearchStrings(strs *[]string, str string) int {
	if strs == nil || len(*strs) == 0 {
		return -1
	}
	if idx := sort.SearchStrings(*strs, str); idx == len(*strs) || (*strs)[idx] != str {
		return -1
	} else {
		return idx
	}
}

func NewMultiError(errs *[]error) *MultiError {
	return &MultiError{
		Errs: *errs,
	}
}

type MultiError struct {
	Errs []error
}

func (e *MultiError) Error() string {
	sbr := strings.Builder{}
	for _, v := range e.Errs {
		sbr.WriteString(v.Error())
		sbr.WriteRune(';')
	}
	return sbr.String()
}
