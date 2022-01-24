package gobus

import (
	"fmt"
	"reflect"
	"strings"
)

/**
 *
 * bus: 总线
 * data: 订阅集合
 * verify: true: 发现错误，立即返回
 * tag: 订阅标签
 */
func SubscribeTag(bus Bus, data interface{}, verify bool, tag string) (func(), error) {
	if tag == "" {
		tag = "gbus"
	}

	clss := []func(){}
	errs := []error{}
	typ := reflect.TypeOf(data)
	val := reflect.ValueOf(data)
	if typ.Kind() == reflect.Ptr { // 指针
		typ = typ.Elem()
		val = val.Elem()
	}
	for k := 0; k < typ.NumField(); k++ {
		tag := typ.Field(k).Tag.Get(tag)
		if tag == "" {
			continue
		}
		// *,com.suisrc.", "*"=>必须使用配置文件订阅
		// Exe00=com.suisrc.topicA#groupA>>com.suisrc.topicB，不读取配置
		// Exe11=, => Exe11=$[method]
		// Exe21=$topics.exe11，读取配置文件
		// Exe31=$topics.exe11,com.suisrc.topicA#groupA
		// Exe41=$,com.suisrc.topicA#groupA

		all := tag[0] == '*'
		cfs := make(map[string]string)
		if !all {
			for _, v := range strings.Split(tag, ";") {
				if v == "" {
					continue
				}
				v2 := strings.SplitN(v, "=", 2)
				if len(v2) == 1 || v2[1] == "" {
					cfs[v2[0]] = "$"
				} else {
					cfs[v2[0]] = v2[1]
				}
			}
		}
		tfv := val.Field(k)
		tft := tfv.Type()
		for name, conf := range cfs {
			_, exsit := tft.MethodByName(name)
			if !exsit {
				continue // 方法不存在跳过
			}
			topic := ""
			if all { // 全部方法都需要订阅
				topic = "$" + strings.ToLower(name)
				if len(tag) > 1 { // 默认主题
					topic += fmt.Sprintf("%s%s#%s", tag[:1], topic[1:], topic[1:])
				}
			} else if conf == "" || conf[0] != '$' {
				topic = conf
			} else if cs := strings.SplitN(conf[1:], ",", 2); cs[0] != "" {
				topic = conf
			} else {
				topic = "$" + strings.ToLower(name) + conf[1:]
			}
			if topic == "" {
				continue
			}
			fn := tfv.MethodByName(name).Interface()
			err := bus.Subscribe(topic, fn)
			if err == ErrNon {
				continue
			} else if err != nil && verify {
				return nil, err
			} else if err != nil {
				errs = append(errs, err)
			} else {
				clss = append(clss, func() { bus.Unsubscribe(topic, fn) })
			}
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
