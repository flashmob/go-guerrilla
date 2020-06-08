package backends

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSetProcessorValue(t *testing.T) {

	var test BackendConfig
	test = make(map[string]map[string]interface{}, 0)
	test.SetValue("processors", "ABC", "key", "value")
	out, _ := json.MarshalIndent(test, "", "   ")
	fmt.Println(string(out))
}
