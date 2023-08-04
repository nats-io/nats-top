package toputils

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	server_test "github.com/nats-io/nats-server/v2/test"
)

func runMonitorServer() *server.Server {
	resetPreviousHTTPConnections()
	opts := server_test.DefaultTestOptions
	// Use random ports.
	opts.Port = -1
	opts.HTTPPort = -1

	return server_test.RunServer(&opts)
}

func resetPreviousHTTPConnections() {
	http.DefaultTransport = &http.Transport{}
}

// retryUntil keeps calling the function f until it returns true or the deadline d has been reached.
func retryUntil(d time.Duration, f func() bool) bool {
	deadline := time.Now().Add(d)

	for time.Now().Before(deadline) {
		if f() {
			return true
		}
	}

	return false
}

func TestFetchingStatz(t *testing.T) {
	srv := runMonitorServer()
	defer srv.Shutdown()

	host := srv.MonitorAddr().IP.String()
	port := srv.MonitorAddr().Port

	engine := NewEngine(host, port, 10, 1)
	engine.SetupHTTP()

	var varz *server.Varz
	result, err := engine.Request("/varz")
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

	connected := make(chan struct{}) // Used to signal that the client has connected.
	done := make(chan struct{})      // Used to exit the client goroutine.
	defer close(done)

	// Create simple subscription to nats-server test port to show subscriptions
	go func() {
		conn, err := net.Dial("tcp", strings.TrimPrefix(srv.ClientURL(), "nats://"))
		if err != nil {
			t.Errorf("could not create subcription to NATS: %s", err)
			return
		}
		defer conn.Close()

		fmt.Fprintf(conn, "SUB hello.world  90\r\n")
		close(connected)
		<-done
	}()

	// Wait for the client to connect.
	select {
	case <-connected:
		t.Log("client connected successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect to the server in time")
	}

	var connz *server.Connz

	// Keep trying to get the connections.
	gotConns := retryUntil(2*time.Second, func() bool {
		result, err = engine.Request("/connz")
		if err != nil {
			t.Fatalf("Failed getting /connz: %v", err)
		}

		if connzVal, ok := result.(*server.Connz); ok {
			connz = connzVal
		}

		return len(connz.Conns) > 0
	})

	if !gotConns {
		t.Fatal("server did not get any connections in time")
	}

	// Check that we got exactly 1 connection
	got = len(connz.Conns)
	if got != 1 {
		t.Fatalf("Could not monitor with subscriptions option. expected 1 conns, got: %v", got)
	}

	engine.DisplaySubs = true
	result, err = engine.Request("/connz")
	if err != nil {
		t.Fatalf("Failed getting /connz: %v", err)
	}

	if connzVal, ok := result.(*server.Connz); ok {
		connz = connzVal
	}

	// Check that we got subscriptions
	got = len(connz.Conns[0].Subs)
	if got != 1 {
		t.Fatalf("Could not monitor with client subscriptions. expected client with subscriptions, got: %v", got)
	}
}

func TestPsize(t *testing.T) {

	type Args struct {
		displayRawBytes bool
		input           int64
	}

	testcases := map[string]struct {
		args Args
		want string
	}{
		"given input 1023 and display_raw_bytes false": {
			args: Args{
				input:           int64(1023),
				displayRawBytes: false,
			},
			want: "1023",
		},
		"given input kibibyte and display_raw_bytes false": {
			args: Args{
				input:           int64(kibibyte),
				displayRawBytes: false,
			},
			want: "1.0K",
		},
		"given input mebibyte and display_raw_bytes false": {
			args: Args{
				input:           int64(mebibyte),
				displayRawBytes: false,
			},
			want: "1.0M",
		},
		"given input gibibyte and display_raw_bytes false": {
			args: Args{
				input:           int64(gibibyte),
				displayRawBytes: false,
			},
			want: "1.0G",
		},

		"given input 1023 and display_raw_bytes true": {
			args: Args{
				input:           int64(1023),
				displayRawBytes: true,
			},
			want: "1023",
		},
		"given input kibibyte and display_raw_bytes true": {
			args: Args{
				input:           int64(kibibyte),
				displayRawBytes: true,
			},
			want: fmt.Sprintf("%d", kibibyte),
		},
		"given input mebibyte and display_raw_bytes true": {
			args: Args{
				input:           int64(mebibyte),
				displayRawBytes: true,
			},
			want: fmt.Sprintf("%d", mebibyte),
		},
		"given input gibibyte and display_raw_bytes true": {
			args: Args{
				input:           int64(gibibyte),
				displayRawBytes: true,
			},
			want: fmt.Sprintf("%d", gibibyte),
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			got := Psize(testcase.args.displayRawBytes, testcase.args.input)

			if got != testcase.want {
				t.Errorf("wanted %q, got %q", testcase.want, got)
			}
		})
	}
}

func TestNsize(t *testing.T) {

	type Args struct {
		displayRawBytes bool
		input           int64
	}

	testcases := map[string]struct {
		args Args
		want string
	}{
		"given input 999 and display_raw_bytes false": {
			args: Args{
				input:           int64(999),
				displayRawBytes: false,
			},
			want: "999",
		},
		"given input 1000 and display_raw_bytes false": {
			args: Args{
				input:           int64(1000),
				displayRawBytes: false,
			},
			want: "1.0K",
		},
		"given input 1_000_000 and display_raw_bytes false": {
			args: Args{
				input:           int64(1_000_000),
				displayRawBytes: false,
			},
			want: "1.0M",
		},
		"given input 1_000_000_000 and display_raw_bytes false": {
			args: Args{
				input:           int64(1_000_000_000),
				displayRawBytes: false,
			},
			want: "1.0B",
		},
		"given input 1_000_000_000_000 and display_raw_bytes false": {
			args: Args{
				input:           int64(1_000_000_000_000),
				displayRawBytes: false,
			},
			want: "1.0T",
		},

		"given input 999 and display_raw_bytes true": {
			args: Args{
				input:           int64(999),
				displayRawBytes: true,
			},
			want: "999",
		},
		"given input 1000 and display_raw_bytes true": {
			args: Args{
				input:           int64(1000),
				displayRawBytes: true,
			},
			want: "1000",
		},
		"given input 1_000_000 and display_raw_bytes true": {
			args: Args{
				input:           int64(1_000_000),
				displayRawBytes: true,
			},
			want: "1000000",
		},
		"given input 1_000_000_000 and display_raw_bytes true": {
			args: Args{
				input:           int64(1_000_000_000),
				displayRawBytes: true,
			},
			want: "1000000000",
		},
		"given input 1_000_000_000_000 and display_raw_bytes true": {
			args: Args{
				input:           int64(1_000_000_000_000),
				displayRawBytes: true,
			},
			want: "1000000000000",
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			got := Nsize(testcase.args.displayRawBytes, testcase.args.input)

			if got != testcase.want {
				t.Errorf("wanted %q, got %q", testcase.want, got)
			}
		})
	}
}

func TestMonitorStats(t *testing.T) {
	srv := runMonitorServer()
	defer srv.Shutdown()

	host := srv.MonitorAddr().IP.String()
	port := srv.MonitorAddr().Port

	engine := NewEngine(host, port, 10, 1)
	engine.SetupHTTP()

	go func() {
		err := engine.MonitorStats()
		if err != nil {
			t.Errorf("Could not start info monitoring loop. expected no error, got: %v", err)
		}
	}()
	defer close(engine.ShutdownCh)

	select {
	case stats := <-engine.StatsCh:
		got := stats.Varz.Cores
		if got < 1 {
			t.Fatalf("Could not monitor number of cores. got: %v", got)
		}
		return
	case <-time.After(3 * time.Second):
		t.Fatalf("Timed out polling /varz via http")
	}
}

func TestMonitoringTLSConnectionUsingRootCA(t *testing.T) {
	srv, _ := server_test.RunServerWithConfig("./test/tls.conf")
	defer srv.Shutdown()

	host := srv.MonitorAddr().IP.String()
	port := srv.MonitorAddr().Port

	engine := NewEngine(host, port, 10, 1)
	err := engine.SetupHTTPS("./test/ca.pem", "", "", false)
	if err != nil {
		t.Fatalf("Expected to be able to configure polling via HTTPS. Got: %s", err)
	}

	go func() {
		err := engine.MonitorStats()
		if err != nil {
			t.Errorf("Could not start info monitoring loop. expected no error, got: %v", err)
		}
	}()
	defer close(engine.ShutdownCh)

	select {
	case stats := <-engine.StatsCh:
		got := stats.Varz.Cores
		if got < 1 {
			t.Fatalf("Could not monitor number of cores. got: %v", got)
		}
		return
	case <-time.After(3 * time.Second):
		t.Fatalf("Timed out polling /varz via https")
	}
}

func TestMonitoringTLSConnectionUsingRootCAWithCerts(t *testing.T) {
	srv, _ := server_test.RunServerWithConfig("./test/tls.conf")
	defer srv.Shutdown()

	host := srv.MonitorAddr().IP.String()
	port := srv.MonitorAddr().Port

	engine := NewEngine(host, port, 10, 1)
	err := engine.SetupHTTPS("./test/ca.pem", "./test/client-cert.pem", "./test/client-key.pem", false)
	if err != nil {
		t.Fatalf("Expected to be able to configure polling via HTTPS. Got: %s", err)
	}

	go func() {
		err := engine.MonitorStats()
		if err != nil {
			t.Errorf("Could not start info monitoring loop. expected no error, got: %v", err)
		}
	}()
	defer close(engine.ShutdownCh)

	select {
	case stats := <-engine.StatsCh:
		got := stats.Varz.Cores
		if got < 1 {
			t.Fatalf("Could not monitor number of cores. got: %v", got)
		}
		return
	case <-time.After(3 * time.Second):
		t.Fatalf("Timed out polling /varz via https")
	}
}

func TestMonitoringTLSConnectionUsingCertsAndInsecure(t *testing.T) {
	srv, _ := server_test.RunServerWithConfig("./test/tls.conf")
	defer srv.Shutdown()

	host := srv.MonitorAddr().IP.String()
	port := srv.MonitorAddr().Port

	engine := NewEngine(host, port, 10, 1)
	err := engine.SetupHTTPS("", "./test/client-cert.pem", "./test/client-key.pem", true)
	if err != nil {
		t.Fatalf("Expected to be able to configure polling via HTTPS. Got: %s", err)
	}

	go func() {
		err := engine.MonitorStats()
		if err != nil {
			t.Errorf("Could not start info monitoring loop. expected no error, got: %v", err)
		}
	}()
	defer close(engine.ShutdownCh)

	select {
	case stats := <-engine.StatsCh:
		got := stats.Varz.Cores
		if got < 1 {
			t.Fatalf("Could not monitor number of cores. got: %v", got)
		}
		return
	case <-time.After(3 * time.Second):
		t.Fatalf("Timed out polling /varz via https")
	}
}
