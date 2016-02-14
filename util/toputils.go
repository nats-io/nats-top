package toputils

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	gnatsd "github.com/nats-io/gnatsd/server"
)

const DisplaySubscriptions = 1

type Engine struct {
	Host        string
	Port        int
	HttpClient  *http.Client
	Uri         string
	Conns       int
	SortOpt     gnatsd.SortOpt
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
		statz = &gnatsd.Varz{}
	case "/connz":
		statz = &gnatsd.Connz{}
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

// MonitorStats is ran as a goroutine and takes options
// which can modify how poll values then sends to channel.
func (engine *Engine) MonitorStats() error {
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

	delay := time.Duration(engine.Delay) * time.Second

	for {
		stats := &Stats{
			Varz:  &gnatsd.Varz{},
			Connz: &gnatsd.Connz{},
			Rates: &Rates{},
			Error: fmt.Errorf(""),
		}

		select {
		case <-engine.ShutdownCh:
			return nil
		case <-time.After(delay):
			// Get /varz
			{
				result, err := engine.Request("/varz")
				if err != nil {
					stats.Error = err
					engine.StatsCh <- stats
					continue
				}
				if varz, ok := result.(*gnatsd.Varz); ok {
					stats.Varz = varz
				}
			}

			// Get /connz
			{
				result, err := engine.Request("/connz")
				if err != nil {
					stats.Error = err
					engine.StatsCh <- stats
					continue
				}
				if connz, ok := result.(*gnatsd.Connz); ok {
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

			engine.StatsCh <- stats
		}
	}
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
	Varz  *gnatsd.Varz
	Connz *gnatsd.Connz
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
