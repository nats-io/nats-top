package test

import (
	"net/http"
	"testing"

	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	. "github.com/nats-io/nats-top/util"
)

// Borrowed from gnatsd tests
const GNATSD_PORT = 11422

func runMonitorServer(monitorPort int) *server.Server {
	resetPreviousHTTPConnections()
	opts := gnatsd.DefaultTestOptions
	opts.Host = "127.0.0.1"
	opts.Port = GNATSD_PORT
	opts.HTTPPort = monitorPort

	return gnatsd.RunServer(&opts)
}

func resetPreviousHTTPConnections() {
	http.DefaultTransport = &http.Transport{}
}

func TestFetchingStatz(t *testing.T) {
	params := make(map[string]interface{})
	params["host"] = "127.0.0.1"
	params["port"] = server.DEFAULT_HTTP_PORT

	s := runMonitorServer(server.DEFAULT_HTTP_PORT)
	defer s.Shutdown()

	// Getting Varz
	var varz *server.Varz
	result, err := Request("/varz", params)
	if err != nil {
		t.Fatalf("Failed getting /varz: %v", err)
	}

	if varzVal, ok := result.(*server.Varz); ok {
		varz = varzVal
	}

	// At the very least it is guaranteed that we have one core
	got := varz.Cores
	if got < 1 {
		t.Fatalf("Could not monitor number of cores. got: %v", got)
	}

	var connz *server.Connz
	result, err = Request("/connz", params)
	if err != nil {
		t.Fatalf("Failed getting /connz: %v", err)
	}

	if connzVal, ok := result.(*server.Connz); ok {
		connz = connzVal
	}

	// Check for default value of connections limit
	got = connz.Limit
	if got != 1024 {
		t.Fatalf("Could not monitor limit of connections. got: %v", got)
	}

	s.Shutdown()
}

func TestPsize(t *testing.T) {

	expected := "1023"
	got := Psize(1023)
	if got != expected {
		t.Fatalf("Wrong human readable value. expected: %v, got: %v", expected, got)
	}

	expected = "1.0K"
	got = Psize(1024)
	if got != expected {
		t.Fatalf("Wrong human readable value. expected: %v, got: %v", expected, got)
	}

	expected = "1.0M"
	got = Psize(1024 * 1024)
	if got != expected {
		t.Fatalf("Wrong human readable value. expected: %v, got: %v", expected, got)
	}

	expected = "1.0G"
	got = Psize(1024 * 1024 * 1024)
	if got != expected {
		t.Fatalf("Wrong human readable value. expected: %v, got: %v", expected, got)
	}
}
