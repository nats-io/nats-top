package toputils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nats-io/gnatsd/server"
)

// Takes a path and options, then returns a serialized connz, varz, or routez response
func Request(path string, opts map[string]interface{}) (interface{}, error) {
	var statz interface{}
	uri := fmt.Sprintf("http://%s:%d%s", opts["host"], opts["port"], path)

	switch path {
	case "/varz":
		statz = &server.Varz{}
	case "/connz":
		statz = &server.Connz{}
		uri += fmt.Sprintf("?n=%d&s=%s", opts["conns"], opts["sort"])
	default:
		return nil, fmt.Errorf("invalid path '%s' for stats server", path)
	}

	resp, err := http.Get(uri)
	if err != nil {
		return nil, fmt.Errorf("error fetching %s: %v", path, err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading stat from upstream: %s", err)
	}

	err = json.Unmarshal(body, &statz)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling json: %v", err)
	}

	return statz, nil
}

// Takes a float and returns a human readable string
func Psize(s int64) string {
	size := float64(s)

	if size < 1024 {
		return fmt.Sprintf("%.0f", size)
	} else if size < (1024 * 1024) {
		return fmt.Sprintf("%.1fK", size/1024)
	} else if size < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.1fM", size/1024/1024)
	} else if size >= (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.1fG", size/1024/1024/1024)
	} else {
		return "NA"
	}
}
