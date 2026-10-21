package toputils

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

const DisplaySubscriptions = 1

const requestTimeout = 5 * time.Second

type Engine struct {
	Host         string
	Port         int
	HttpClient   *http.Client
	Uri          string
	Conns        int
	SortOpt      server.SortOpt
	Delay        int
	DisplaySubs  bool
	StatsCh      chan *Stats
	ShutdownCh   chan struct{}
	LastStats    *Stats
	LastPollTime time.Time
	ShowRates    bool
	LastConnz    map[uint64]*server.ConnInfo
}

func NewEngine(host string, port int, conns int, delay int) *Engine {
	return &Engine{
		Host:       host,
		Port:       port,
		Conns:      conns,
		Delay:      delay,
		StatsCh:    make(chan *Stats),
		ShutdownCh: make(chan struct{}),
		LastConnz:  make(map[uint64]*server.ConnInfo),
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		end := bytes.IndexAny(body, "\r\n")
		if end < 0 || end > 80 {
			end = min(len(body), 80)
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
	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	for {
		select {
		case engine.StatsCh <- engine.fetchStats():
		case <-engine.ShutdownCh:
			return nil
		}

		select {
		case <-ticker.C:
		case <-engine.ShutdownCh:
			return nil
		}
	}
}

func (engine *Engine) FetchStatsSnapshot() *Stats {
	return engine.fetchStats()
}

var errDud = fmt.Errorf("")

func (engine *Engine) fetchStats() *Stats {
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
			return stats
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
			return stats
		}

		if connz, ok := result.(*server.Connz); ok {
			stats.Connz = connz
		}
	}

	var isFirstTime bool
	if engine.LastStats != nil {
		inMsgsLastVal = engine.LastStats.Varz.InMsgs
		outMsgsLastVal = engine.LastStats.Varz.OutMsgs
		inBytesLastVal = engine.LastStats.Varz.InBytes
		outBytesLastVal = engine.LastStats.Varz.OutBytes
	} else {
		isFirstTime = true
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

	// Snapshot per sec metrics for connections.
	connz := make(map[uint64]*server.ConnInfo)
	for _, conn := range stats.Connz.Conns {
		connz[conn.Cid] = conn
	}

	// Calculate rates but the first time
	var secs float64
	if !isFirstTime {
		secs = stats.Varz.Now.Sub(engine.LastStats.Varz.Now).Seconds()

		inMsgsRate = float64(inMsgsDelta) / secs
		outMsgsRate = float64(outMsgsDelta) / secs
		inBytesRate = float64(inBytesDelta) / secs
		outBytesRate = float64(outBytesDelta) / secs
	}
	rates := &Rates{
		InMsgsRate:   inMsgsRate,
		OutMsgsRate:  outMsgsRate,
		InBytesRate:  inBytesRate,
		OutBytesRate: outBytesRate,
		Connections:  make(map[uint64]*ConnRates),
	}

	// Measure per connection metrics.
	for cid, conn := range connz {
		cr := &ConnRates{
			InMsgsRate:   0,
			OutMsgsRate:  0,
			InBytesRate:  0,
			OutBytesRate: 0,
		}
		lconn, wasConnected := engine.LastConnz[cid]
		if wasConnected {
			cr.InMsgsRate = float64(conn.InMsgs-lconn.InMsgs) / secs
			cr.OutMsgsRate = float64(conn.OutMsgs-lconn.OutMsgs) / secs
			cr.InBytesRate = float64(conn.InBytes-lconn.InBytes) / secs
			cr.OutBytesRate = float64(conn.OutBytes-lconn.OutBytes) / secs
		}
		rates.Connections[cid] = cr
	}

	stats.Rates = rates

	// Snapshot stats.
	engine.LastStats = stats
	engine.LastPollTime = time.Now()
	engine.LastConnz = connz

	return stats
}

// SetupHTTPS sets up the http client and uri to use for polling.
func (engine *Engine) SetupHTTPS(caCertOpt, certOpt, keyOpt string, skipVerifyOpt bool) error {
	tlsConfig := &tls.Config{}
	if caCertOpt != "" {
		caCert, err := os.ReadFile(caCertOpt)
		if err != nil {
			return err
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("no valid certificates found in %s", caCertOpt)
		}
		tlsConfig.RootCAs = caCertPool
	}

	if (certOpt == "") != (keyOpt == "") {
		return fmt.Errorf("client certificate and key must be provided together")
	}

	if certOpt != "" {
		cert, err := tls.LoadX509KeyPair(certOpt, keyOpt)
		if err != nil {
			return err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if skipVerifyOpt {
		tlsConfig.InsecureSkipVerify = true
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	engine.setup("https", transport)

	return nil
}

// SetupHTTP sets up the http client and uri to use for polling.
func (engine *Engine) SetupHTTP() {
	engine.setup("http", nil)
}

func (engine *Engine) setup(scheme string, transport http.RoundTripper) {
	engine.HttpClient = &http.Client{Transport: transport, Timeout: requestTimeout}
	// Trim brackets so both "::1" and "[::1]" work as IPv6 hosts.
	engine.Uri = scheme + "://" + net.JoinHostPort(strings.Trim(engine.Host, "[]"), strconv.Itoa(engine.Port))
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
	Connections  map[uint64]*ConnRates
}

type ConnRates struct {
	InMsgsRate   float64
	OutMsgsRate  float64
	InBytesRate  float64
	OutBytesRate float64
}

const kibibyte = 1024
const mebibyte = 1024 * 1024
const gibibyte = 1024 * 1024 * 1024

// Psize takes a float and returns a human readable string (Used for bytes).
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

const k = 1000
const m = k * 1000
const b = m * 1000
const t = b * 1000

// Nsize takes a float and returns a human readable string.
func Nsize(displayRawValue bool, s int64) string {
	size := float64(s)

	switch {
	case displayRawValue || size < k:
		return fmt.Sprintf("%.0f", size)
	case size < m:
		return fmt.Sprintf("%.1fK", size/k)
	case size < b:
		return fmt.Sprintf("%.1fM", size/m)
	case size < t:
		return fmt.Sprintf("%.1fB", size/b)
	default:
		return fmt.Sprintf("%.1fT", size/t)
	}
}
