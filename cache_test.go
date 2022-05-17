package readcache

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/beego/beego/v2/client/cache"
)

func TestMarshalString(t *testing.T) {
	d, err := json.Marshal("abc")
	if err != nil {
		t.Error(err)
	}
	t.Log(string(d))

	var str string
	err = json.Unmarshal(d, &str)
	if err != nil {
		t.Error(err)
	}
	t.Log(str)

	// 可以对基本类型进行 json 序列化
}

func TestNilTypeCase(t *testing.T) {
	type Nilable struct {
		name string
	}

	var nilable *Nilable = nil
	func(n interface{}) {
		v, ok := n.(*Nilable)
		if !ok {
			t.Error("error")
		}
		t.Log(v)
	}(nilable)

	// 可以对 nil 进行 comma-ok 类型转换
}

func TestMemoryCacheNil(t *testing.T) {
	memCache, err := cache.NewCache("memory", fmt.Sprintf(`{"interval":60}`))
	if err != nil {
		t.Error(err)
	}

	_, err = memCache.Get(nil, "abc")
	if err == nil {
		t.Error("error")
	}

	// 如果 key 不存在不会返回 nil 而是返回 error：`ERROR-4002015, the key isn't exist`
}
