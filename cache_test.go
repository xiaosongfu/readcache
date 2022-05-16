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
}
