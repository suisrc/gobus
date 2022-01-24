package gobus

import (
	"encoding/json"
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

// 配置主题列表， 在配置主题前需要声明该内容， 默认为空
var (
	Topics = make(map[string]string) // 系统初始化时候， 需要加载配置文件
	ErrNon = fmt.Errorf("topic is empty")
)

type EventSubscriber interface {
	Subscribe(Bus) (func(), error)
}

type EventHandler interface {
	Subscribe() (kind Kind, topic string, handler interface{})
}

//=======================================================================================

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

func GetTopicName(topic string) (string, error) {
	if topic == "" {
		return "", nil
	}
	if topic[0] == '$' {
		keys := strings.SplitN(topic[1:], ",", 2)
		topic = ""
		if keys[0] != "" {
			topic = Topics[keys[0]]
		}
		if topic == "" && len(keys) == 2 { // 使用默认主题
			topic = keys[1]
		}
	}
	if topic == "" {
		return "", ErrNon
	}
	return topic, nil
}

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

//=======================================================================================

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
