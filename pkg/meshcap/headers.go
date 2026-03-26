package meshcap

import (
	"encoding/json"
	"fmt"
)

// MarshalHeaders serializes an HTTP header map to a JSON string.
func MarshalHeaders(h map[string]string) string {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Sprintf("{\"error\": %q}", err.Error())
	}
	return string(data)
}
