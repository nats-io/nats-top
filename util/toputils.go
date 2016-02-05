package toputils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	gnatsd "github.com/nats-io/gnatsd/server"
)

const DisplaySubscriptions = 1

// Request takes a path and options, and returns a Stats struct
// with with either connz or varz
func Request(path string, opts map[string]interface{}) (interface{}, error) {
	var statz interface{}
	uri := fmt.Sprintf("http://%s:%d%s", opts["host"], opts["port"], path)

	switch path {
	case "/varz":
		statz = &gnatsd.Varz{}
	case "/connz":
		statz = &gnatsd.Connz{}
		uri += fmt.Sprintf("?limit=%d&sort=%s", opts["conns"], opts["sort"])
		if displaySubs, ok := opts["subs"]; ok {
			if displaySubs.(bool) {
				uri += fmt.Sprintf("&subs=%d", DisplaySubscriptions)
			}
		}
	default:
		return nil, fmt.Errorf("invalid path '%s' for stats server", path)
	}

	resp, err := http.Get(uri)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("could not get stats from server: %v\n", err)
	}

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

// Psize takes a float and returns a human readable string.
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

// MonitorStats is ran as a goroutine and takes options
// which can modify how poll values then sends to channel.
func MonitorStats(
	opts map[string]interface{},
	statsCh chan *Stats,
	shutdownCh chan struct{},
) error {
	var pollTime time.Time

	var inMsgsDelta int64
	var outMsgsDelta int64
	var inBytesDelta int64
	var outBytesDelta int64

	var inMsgsLastVal int64
	var outMsgsLastVal int64
	var inBytesLastVal int64
	var outBytesLastVal int64

	var inMsgsRate float64
	var outMsgsRate float64
	var inBytesRate float64
	var outBytesRate float64

	first := true
	pollTime = time.Now()

	var delay time.Duration
	if val, ok := opts["delay"].(int); ok {
		delay = time.Duration(val) * time.Second
	} else {
		return fmt.Errorf("error: could not use %s as a refreshing interval", opts["delay"])
	}

	// Wrap collected info in a Stats struct
	stats := &Stats{
		Varz:  &gnatsd.Varz{},
		Connz: &gnatsd.Connz{},
		Rates: &Rates{},
	}

	for {
		select {
		case <-shutdownCh:
			return nil
		case <-time.After(delay):
			// Get /varz
			{
				result, err := Request("/varz", opts)
				if err == nil {
					if varz, ok := result.(*gnatsd.Varz); ok {
						stats.Varz = varz
					}
				}
			}

			// Get /connz
			{
				result, err := Request("/connz", opts)
				if err == nil {
					if connz, ok := result.(*gnatsd.Connz); ok {
						stats.Connz = connz
					}
				}
			}

			// Periodic snapshot to get per sec metrics
			inMsgsVal := stats.Varz.InMsgs
			outMsgsVal := stats.Varz.OutMsgs
			inBytesVal := stats.Varz.InBytes
			outBytesVal := stats.Varz.OutBytes

			inMsgsDelta = inMsgsVal - inMsgsLastVal
			outMsgsDelta = outMsgsVal - outMsgsLastVal
			inBytesDelta = inBytesVal - inBytesLastVal
			outBytesDelta = outBytesVal - outBytesLastVal

			inMsgsLastVal = inMsgsVal
			outMsgsLastVal = outMsgsVal
			inBytesLastVal = inBytesVal
			outBytesLastVal = outBytesVal

			now := time.Now()
			tdelta := now.Sub(pollTime)
			pollTime = now

			// Calculate rates but the first time
			if first {
				first = false
			} else {
				inMsgsRate = float64(inMsgsDelta) / tdelta.Seconds()
				outMsgsRate = float64(outMsgsDelta) / tdelta.Seconds()
				inBytesRate = float64(inBytesDelta) / tdelta.Seconds()
				outBytesRate = float64(outBytesDelta) / tdelta.Seconds()
			}

			stats.Rates = &Rates{
				InMsgsRate:   inMsgsRate,
				OutMsgsRate:  outMsgsRate,
				InBytesRate:  inBytesRate,
				OutBytesRate: outBytesRate,
			}

			statsCh <- stats
		}
	}
}
