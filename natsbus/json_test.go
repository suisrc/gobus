package natsbus_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/suisrc/gobus/natsbus"
)

func TestJson1(t *testing.T) {
	msg := "stringstring"
	bss, err := natsbus.FmtData2Byte([]byte(msg))
	fmt.Println(msg, string(bss), err)
	assert.NotNil(t, nil)
}

func TestJson2(t *testing.T) {
	msg := "'stringstring'"
	bss := []byte{}
	err := json.Unmarshal([]byte(msg), &bss)
	fmt.Println(msg, bss, err)
	assert.NotNil(t, nil)
}

func TestJson3(t *testing.T) {
	msg := `{"name":"123456"}`

	f := reflect.TypeOf(Exec)
	val, err := natsbus.FmtByte2RefVal(f.In(0), []byte(msg))
	fmt.Println(msg, val.Interface(), err)
	reflect.ValueOf(Exec).Call([]reflect.Value{val})
	assert.NotNil(t, nil)
}

func TestJson4(t *testing.T) {
	f := reflect.TypeOf(Exec)
	p := f.In(0)
	fmt.Println(p.String())
	assert.NotNil(t, nil)
}

func Exec(data *ExecP) (string, error) {
	fmt.Printf("======%v\n", data)
	return "hello", nil
}

type ExecP struct {
	Name string `json:"name"`
}
