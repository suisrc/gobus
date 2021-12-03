# 说明

这是一个外部总线的定义，nats只是其中的一种实现方式

## 引用

```go
import (
	"github.com/suisrc/gobus"
	"github.com/suisrc/gobus/natsbus"
)
```

## 例子

```go

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

	bus.Request("test_bus", 2*time.Second, msg, msg)

	str, _ := json.Marshal(msg)
	fmt.Println(string(str))
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

```


## NATS
https://github.com/nats-io/nats.go

## NATS三种模型
https://www.cnblogs.com/yorkyang/p/8392752.html

### 发布/订阅模型
NATS的发布/订阅通信模型是一对多的消息通信。发布者在一个主题上发送消息，任何注册（订阅）了此主题的客户端都可以接收到该主题的消息。订阅者可以使用主题通配符订阅感兴趣的主题。  
对于订阅者，可以选择异步处理或同步处理接收到的消息。如果异步处理消息，消息交付给订阅者的消息句柄。如果客户端没有句柄，那么该消息通信是同步的，那么客户端可能会被阻塞，直到它处理了当前消息。 

#### 服务质量（QoS）
至多发送一次 (TCP reliability)：如果客户端没有注册某个主题（或者客户端不在线），那么该主题发布消息时，客户端不会收到该消息。NATS系统是一种“发送后不管”的消息通信系统，故如果需要高级服务，可以选择"NATS Streaming" 或 在客户端开发相应的功能至少发送一次(NATS Streaming) ：一些使用场景需要更高级更严格的发送保证，这些应用依赖底层传送消息，不论网络是否中断或订阅者是否在线，都要确保订阅者可以收到消息

### 请求/响应模型
NATS支持两种请求-响应消息通信：P2P（点对点）和O2M（一对多）。P2P最快、响应也最先。而对于O2M，需要设置请求者可以接收到的响应数量界限（默认只能收到一条来自订阅者的响应——随机）在请求-响应模式，发布请求操作会发布一个带预期响应的消息到Reply主题。请求创建了一个收件箱，并在收件箱执行调用，并进行响应和返回。多个订阅者(reply 例子)订阅了同一个 主题，请求者向该主题发送一个请求，默认只收到一个订阅者的响应(随机)

事实上，NATS协议中并没有定义 “请求” 或 "响应"方法，它是通过 SUB/PUB变相实现的：请求者先 通过SUB创建一个收件箱，然后发送一个带 reply-to 的PUB，响应者收到PUB消息后，向 reply-to 发送 响应消息，从而实现 请求/响应。reply-to和收件箱都是一个 subject，前者是后者的子集（O2M的情况）

### 队列模型
NATS支持P2P消息通信的队列。要创建一个消息队列，订阅者需注册一个队列名。所有的订阅者用同一个队列名，形成一个队列组。当消息发送到主题后，队列组会自动选择一个成员接收消息。尽管队列组有多个订阅者，但每条消息只能被组中的一个订阅者接收。队列的订阅者可以是异步的，这意味着消息句柄以回调方式处理交付的消息。同步队列订阅者必须建立处理消息的逻辑，NATS支持P2P消息通信的队列。要创建一个消息队列，订阅者需注册一个队列名。所有的订阅者用同一个队列名，形成一个队列组。当消息发送到主题后，队列组会自动选择一个成员接收消息。尽管队列组有多个订阅者，但每条消息只能被组中的一个订阅者接收。队列的订阅者可以是异步的，这意味着消息句柄以回调方式处理交付的消息。异步队列订阅者必须建立处理消息的逻辑。队列模型一般常用于数据队列使用，例如：从网页上采集的数据经过处理直接写入到该队列，接收端一方可以起多个线程同时读取其中的一个队列，其中某些数据被一个线程消费了，其他线程就看不到了，这种方式为了解决采集量巨大的情况下，后端服务可以动态调整并发数来消费这些数据。说白了就一点，上游生产数据太快，下游消费可能处理不过来，中间进行缓冲，下游就可以根据实际情况进行动态调整达到动态平衡。


### 例子

```go
import "github.com/nats-io/nats.go"

// Connect to a server
nc, _ := nats.Connect(nats.DefaultURL)

// Simple Publisher
nc.Publish("foo", []byte("Hello World"))

// Simple Async Subscriber
nc.Subscribe("foo", func(m *nats.Msg) {
    fmt.Printf("Received a message: %s\n", string(m.Data))
})

// Responding to a request message
nc.Subscribe("request", func(m *nats.Msg) {
    m.Respond([]byte("answer is 42"))
})

// Simple Sync Subscriber
sub, err := nc.SubscribeSync("foo")
m, err := sub.NextMsg(timeout)

// Channel Subscriber
ch := make(chan *nats.Msg, 64)
sub, err := nc.ChanSubscribe("foo", ch)
msg := <- ch

// Unsubscribe
sub.Unsubscribe()

// Drain
sub.Drain()

// Requests
msg, err := nc.Request("help", []byte("help me"), 10*time.Millisecond)

// Replies
nc.Subscribe("help", func(m *nats.Msg) {
    nc.Publish(m.Reply, []byte("I can help!"))
})

// Drain connection (Preferred for responders)
// Close() not needed if this is called.
nc.Drain()

// Close connection
nc.Close()
```