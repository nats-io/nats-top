package test

import (
	"testing"

	"github.com/nats-io/gnatsd/server"
	natstest "github.com/nats-io/gnatsd/test"
	. "github.com/nats-io/nats-top/util"
)

func TestFetchingStatz(t *testing.T) {
	params := make(map[string]interface{})
	natsHttpPort := 8222

	params["host"] = "127.0.0.1"
	params["port"] = natsHttpPort

	opts := natstest.DefaultTestOptions
	opts.Port = 8888
	opts.HTTPPort = natsHttpPort

	s := natstest.RunServer(&opts)

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

	// At the very least it is guaranteed that we have one core
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
