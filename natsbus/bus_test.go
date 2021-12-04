package natsbus_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	gnatsd "github.com/nats-io/gnatsd/test"
	"github.com/nats-io/go-nats"
	"github.com/stretchr/testify/assert"
	"github.com/suisrc/gobus"
	"github.com/suisrc/gobus/natsbus"
)

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

	gobus.SubscribeBatch(bus, &inj, false, "Bus")

	bus.Publish("test_bus", "helloword")

	res, _ := bus.RequestB("test_bus", 2*time.Second, "hello, go")
	fmt.Println(string(res))
	assert.NotNil(t, nil)
}

type Inject struct {
	Bus      gobus.Bus
	SubDemo  SubDemo
	SubDemo2 SubDemo2
}

var _ gobus.EventHandler = (*SubDemo)(nil)

type SubDemo struct {
}

func (s SubDemo) Subscribe() (kind gobus.Kind, topic string, handler interface{}) {
	return gobus.BusSync, "test_bus", s.exec1
}

func (*SubDemo) exec1(msg string) (string, error) {
	return "hello, gobus", nil
}

type SubDemo2 struct {
}

func (s SubDemo2) Subscribe() (kind gobus.Kind, topic string, handler interface{}) {
	return gobus.BusAsync, "test_bus", s.exec2
}

func (*SubDemo2) exec2(msg string) {
	fmt.Println(msg)
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
