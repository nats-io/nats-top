package toputils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nats-io/gnatsd/server"
)

type ByCid []*server.ConnInfo

func (d ByCid) Len() int {
	return len(d)
}
func (d ByCid) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByCid) Less(i, j int) bool {
	return d[i].Cid < d[j].Cid
}

type BySubs []*server.ConnInfo

func (d BySubs) Len() int {
	return len(d)
}
func (d BySubs) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d BySubs) Less(i, j int) bool {
	return d[i].NumSubs < d[j].NumSubs
}

type ByPending []*server.ConnInfo

func (d ByPending) Len() int {
	return len(d)
}
func (d ByPending) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByPending) Less(i, j int) bool {
	return d[i].Pending < d[j].Pending
}

type ByMsgsTo []*server.ConnInfo

func (d ByMsgsTo) Len() int {
	return len(d)
}
func (d ByMsgsTo) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByMsgsTo) Less(i, j int) bool {
	return d[i].OutMsgs < d[j].OutMsgs
}

type ByMsgsFrom []*server.ConnInfo

func (d ByMsgsFrom) Len() int {
	return len(d)
}
func (d ByMsgsFrom) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByMsgsFrom) Less(i, j int) bool {
	return d[i].InMsgs < d[j].InMsgs
}

type ByBytesTo []*server.ConnInfo

func (d ByBytesTo) Len() int {
	return len(d)
}
func (d ByBytesTo) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByBytesTo) Less(i, j int) bool {
	return d[i].OutBytes < d[j].OutBytes
}

type ByBytesFrom []*server.ConnInfo

func (d ByBytesFrom) Len() int {
	return len(d)
}
func (d ByBytesFrom) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByBytesFrom) Less(i, j int) bool {
	return d[i].InBytes < d[j].InBytes
}

// Takes a path and options, then returns a serialized connz, varz, or routez response
func Request(path string, opts map[string]interface{}) (interface{}, error) {
	var statz interface{}
	uri := fmt.Sprintf("http://%s:%d%s", opts["host"], opts["port"], path)

	switch path {
	case "/varz":
		statz = &server.Varz{}
	case "/connz":
		statz = &server.Connz{}
		uri += fmt.Sprintf("?limit=%d&s=%s", opts["conns"], opts["sort"])
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
