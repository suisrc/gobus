package natsbus_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	gnatsd "github.com/nats-io/gnatsd/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/suisrc/gobus"
	"github.com/suisrc/gobus/natsbus"
)

// topic => topicA#groupA>>topicB;, 从主题A订阅转发到主题B, 异步订阅专用
// Once订阅只支持无分组订阅, 同步订阅不支持转发
//
// $[默认方法名]?[默认配置内容],...
//
// 1.没有返回值， 异步订阅
// 2.有返回值，并且订阅包含=>异步订阅
// 3.否则同步订阅
// 4.单次订阅不指定标签注入
// 5.标签注入默认场景下全部是异步订阅，以防止未配置导致外部总线的破坏
type Inject struct {
	Bus      gobus.Bus
	SubDemo  SubDemo  `gbus:"Exec1"` // Exec1=$?test_bus>>test_bus2,Exec1=$exec?test_bus
	SubDemo2 SubDemo2 `gbus:"Exec2=test_bus2,Exec4"`
}

var _ gobus.EventHandler = (*SubDemo)(nil)

type SubDemo struct {
}

func (s SubDemo) Subscribe() (kind gobus.Kind, topic string, handler interface{}) {
	return gobus.BusSync, "test_bus", s.Exec1
}

func (SubDemo) Exec1(msg string) (string, error) {
	return msg + "gobus", nil
}

type SubDemo2 struct {
}

func (s SubDemo2) Subscribe() (kind gobus.Kind, topic string, handler interface{}) {
	return gobus.BusAsync, "test_bus", s.Exec2
}

func (SubDemo2) Exec2(msg string) {
	fmt.Println(msg + " exec2")
}

func (SubDemo2) Exec3(msg string) {
	fmt.Println(msg + " exec3")
}

//=================================================================================================

func TestBus(t *testing.T) {
	// Setup nats server
	s := gnatsd.RunDefaultServer()
	defer s.Shutdown()

	endpoint := fmt.Sprintf("nats://localhost:%d", nats.DefaultPort)
	nc, _ := nats.Connect(endpoint)

	bus := natsbus.New(nc)
	inj := Inject{
		bus,
		SubDemo{},
		SubDemo2{},
	}

	gobus.SubscribeBatch(bus, inj, false, "Bus")
	bus.Publish("test_bus", "helloword")

	res, _ := bus.RequestB("test_bus", 2*time.Second, "hello, ")
	fmt.Println(string(res))
	assert.NotNil(t, nil)
}

func TestBusTag(t *testing.T) {
	// Setup nats server
	s := gnatsd.RunDefaultServer()
	defer s.Shutdown()

	endpoint := fmt.Sprintf("nats://localhost:%d", nats.DefaultPort)
	nc, _ := nats.Connect(endpoint)

	bus := natsbus.New(nc)
	inj := Inject{
		bus,
		SubDemo{},
		SubDemo2{},
	}

	gobus.Topics = map[string]string{
		"exec":  "test_bus#T123>>test_bus2",
		"exec1": "test_bus#T123",
		"exec3": "test_bus#T123",
	}
	_, err := gobus.SubscribeTag(bus, inj, false, "")
	assert.Nil(t, err)

	res, _ := bus.RequestB("test_bus", 2*time.Second, "hello ,")
	fmt.Println(string(res))
	assert.NotNil(t, nil)
}

//=================================================================================================

func TestBus2(t *testing.T) {
	// Setup nats server
	s := gnatsd.RunDefaultServer()
	defer s.Shutdown()

	endpoint := fmt.Sprintf("nats://localhost:%d", nats.DefaultPort)
	nc, _ := nats.Connect(endpoint)

	bus := natsbus.New(nc)
	inj := Inject2{
		bus,
		Sub2Demo{},
	}

	gobus.SubscribeBatch(bus, &inj, false, "Bus")

	msg := &Sub2Data{
		Name: "hello",
	}

	err := bus.Request("test_bus", time.Second, msg, msg)

	str, _ := json.Marshal(msg)
	fmt.Println(string(str), err)
	assert.NotNil(t, nil)
}

type Inject2 struct {
	Bus      gobus.Bus
	Sub2Demo Sub2Demo
}

var _ gobus.EventHandler = (*SubDemo)(nil)

type Sub2Demo struct {
}

func (s Sub2Demo) Subscribe() (kind gobus.Kind, topic string, handler interface{}) {
	return gobus.BusSync, "test_bus", s.exec1
}

func (*Sub2Demo) exec1(msg *Sub2Data) (*Sub2Data, error) {
	fmt.Println("name: " + msg.Name)
	msg.Name += "world"
	return msg, nil
}

type Sub2Data struct {
	Name string
}
