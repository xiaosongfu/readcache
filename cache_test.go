package readcache

import (
	"encoding/json"
	"testing"
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
}
