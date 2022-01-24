package gobus

import "time"

//BusSubscriber defines subscription-related bus behavior
// topic => topicA/groupA>>topicB, 从主题A订阅转发到主题B, 异步订阅专用
// Once订阅只支持无分组订阅, 同步订阅不支持转发
type BusSubscriber interface {
	Subscribe(topic string, fn interface{}) error          // 同步订阅 func(?) ?(,?)
	SubscribeAsync(topic string, fn interface{}) error     // 异步订阅 func(?)(?(,?))
	Unsubscribe(topic string, fn interface{}) error        // 取消订阅
	SubscribeOnce(topic string, fn interface{}) error      // 同步一次订阅 func(?) ?(,?)
	SubscribeOnceAsync(topic string, fn interface{}) error // 异步一次订阅 func(?)(?(,?))
}

//BusPublisher defines publishing-related bus behavior
type BusPublisher interface {
	Publish(topic string, args interface{}) error                                            // 推送
	Request(topic string, timeout time.Duration, args interface{}, result interface{}) error // 推送，同步结果
	RequestB(topic string, timeout time.Duration, args interface{}) ([]byte, error)          // 推送，同步结果
}

//Bus englobes global (subscribe, publish, control) bus behavior
type Bus interface {
	BusSubscriber
	BusPublisher
}
