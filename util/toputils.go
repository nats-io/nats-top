package toputils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nats-io/gnatsd/server"
)

// Request takes a path and options, and returns a Stats struct
// with with either connz or varz
func Request(path string, opts map[string]interface{}) (interface{}, error) {
	var statz interface{}
	uri := fmt.Sprintf("http://%s:%d%s", opts["host"], opts["port"], path)

	switch path {
	case "/varz":
		statz = &server.Varz{}
	case "/connz":
		statz = &server.Connz{}
		uri += fmt.Sprintf("?limit=%d&sort=%s", opts["conns"], opts["sort"])
	default:
		return nil, fmt.Errorf("invalid path '%s' for stats server", path)
	}

	resp, err := http.Get(uri)
	if err != nil {
		return nil, fmt.Errorf("could not get stats from server: %v\n", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v\n", err)
	}

	err = json.Unmarshal(body, &statz)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal json: %v\n", err)
	}

	return statz, nil
}

// Psize takes a float and returns a human readable string
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
