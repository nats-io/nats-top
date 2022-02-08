package toputils

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

const DisplaySubscriptions = 1

type Engine struct {
	Host        string
	Port        int
	HttpClient  *http.Client
	Uri         string
	Conns       int
	SortOpt     server.SortOpt
	Delay       int
	DisplaySubs bool
	StatsCh     chan *Stats
	ShutdownCh  chan struct{}
}

func NewEngine(host string, port int, conns int, delay int) *Engine {
	return &Engine{
		Host:       host,
		Port:       port,
		Conns:      conns,
		Delay:      delay,
		StatsCh:    make(chan *Stats),
		ShutdownCh: make(chan struct{}),
	}
}

// Request takes a path and options, and returns a Stats struct
// with with either connz or varz
func (engine *Engine) Request(path string) (interface{}, error) {
	var statz interface{}

	uri := engine.Uri + path
	switch path {
	case "/varz":
		statz = &server.Varz{}
	case "/connz":
		statz = &server.Connz{}
		uri += fmt.Sprintf("?limit=%d&sort=%s", engine.Conns, engine.SortOpt)
		if engine.DisplaySubs {
			uri += fmt.Sprintf("&subs=%d", DisplaySubscriptions)
		}
	default:
		return nil, fmt.Errorf("invalid path '%s' for stats server", path)
	}

	resp, err := engine.HttpClient.Get(uri)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("could not get stats from server: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		end := bytes.IndexAny(body, "\r\n")
		if end > 80 {
			end = 80
		}
		return nil, fmt.Errorf("stats request failed %d: %q", resp.StatusCode, string(body[:end]))
	}

	err = json.Unmarshal(body, &statz)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal statz json: %w", err)
	}

	return statz, nil
}

// MonitorStats is ran as a goroutine and takes options
// which can modify how poll values then sends to channel.
func (engine *Engine) MonitorStats() error {
	delay := time.Duration(engine.Delay) * time.Second
	isFirstTime := true
	lastPollTime := time.Now()

	for {
		select {
		case <-engine.ShutdownCh:
			return nil
		case <-time.After(delay):
			stats, newLastPollTime := engine.fetchStats(isFirstTime, lastPollTime)
			if stats != nil && errors.Is(stats.Error, errDud) {
				isFirstTime = false
				lastPollTime = newLastPollTime
			}

			engine.StatsCh <- stats
		}
	}
}

func (engine *Engine) FetchStatsSnapshot() *Stats {
	stats, _ := engine.fetchStats(true, time.Now())

	return stats
}

var errDud = fmt.Errorf("")

func (engine *Engine) fetchStats(isFirstTime bool, lastPollTime time.Time) (*Stats, time.Time) {
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

	stats := &Stats{
		Varz:  &server.Varz{},
		Connz: &server.Connz{},
		Rates: &Rates{},
		Error: errDud,
	}

	// Get /varz
	{
		result, err := engine.Request("/varz")
		if err != nil {
			stats.Error = err
			return stats, time.Time{}
		}

		if varz, ok := result.(*server.Varz); ok {
			stats.Varz = varz
		}
	}

	// Get /connz
	{
		result, err := engine.Request("/connz")
		if err != nil {
			stats.Error = err
			return stats, time.Time{}
		}

		if connz, ok := result.(*server.Connz); ok {
			stats.Connz = connz
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
	tdelta := now.Sub(lastPollTime)

	// Calculate rates but the first time
	if !isFirstTime {
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

	return stats, now
}

// SetupHTTPS sets up the http client and uri to use for polling.
func (engine *Engine) SetupHTTPS(caCertOpt, certOpt, keyOpt string, skipVerifyOpt bool) error {
	tlsConfig := &tls.Config{}
	if caCertOpt != "" {
		caCert, err := ioutil.ReadFile(caCertOpt)
		if err != nil {
			return err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	if certOpt != "" && keyOpt != "" {
		cert, err := tls.LoadX509KeyPair(certOpt, keyOpt)
		if err != nil {
			return err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if skipVerifyOpt {
		tlsConfig.InsecureSkipVerify = true
	}

	transport := &http.Transport{TLSClientConfig: tlsConfig}
	engine.HttpClient = &http.Client{Transport: transport}
	engine.Uri = fmt.Sprintf("https://%s:%d", engine.Host, engine.Port)

	return nil
}

// SetupHTTP sets up the http client and uri to use for polling.
func (engine *Engine) SetupHTTP() {
	engine.HttpClient = &http.Client{}
	engine.Uri = fmt.Sprintf("http://%s:%d", engine.Host, engine.Port)

	return
}

// Stats represents the monitored data from a NATS server.
type Stats struct {
	Varz  *server.Varz
	Connz *server.Connz
	Rates *Rates
	Error error
}

// Rates represents the tracked in/out msgs and bytes flow
// from a NATS server.
type Rates struct {
	InMsgsRate   float64
	OutMsgsRate  float64
	InBytesRate  float64
	OutBytesRate float64
}

const kibibyte = 1024
const mebibyte = 1024 * 1024
const gibibyte = 1024 * 1024 * 1024

// Psize takes a float and returns a human readable string.
func Psize(displayRawValue bool, s int64) string {
	size := float64(s)

	if displayRawValue || size < kibibyte {
		return fmt.Sprintf("%.0f", size)
	}

	if size < mebibyte {
		return fmt.Sprintf("%.1fK", size/kibibyte)
	}

	if size < gibibyte {
		return fmt.Sprintf("%.1fM", size/mebibyte)
	}

	return fmt.Sprintf("%.1fG", size/gibibyte)
}
