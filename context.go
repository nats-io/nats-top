package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/nats-io/jsm.go/natscontext"
)

// withContext will load a context and set it's setting to the global
// flag values: host, certOpt, keyOpt, caCertOpt
func withContext(name string) {
	if name == "" {
		name = natscontext.SelectedContext()
	}
	if name == "" {
		fmt.Fprintf(os.Stderr, "nats-top: context name required\n")
		return
	}

	natsCtx, err := natscontext.New(name, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats-top: error loading context: %s\n", err)
		return
	}

	// set the global flag values
	*host = extractNatsHost(natsCtx.ServerURL())
	*certOpt = natsCtx.Certificate()
	*keyOpt = natsCtx.Key()
	*caCertOpt = natsCtx.CA()
}

// serverURL transform nats//127.0.0.1:4222,nats://127.0.0.2:4222 to
// 127.0.0.1 hostname only for nats-top
func extractNatsHost(serverURL string) (host string) {
	if serverURL == "" {
		return
	}

	arr := strings.Split(serverURL, ",")
	if len(arr) == 0 {
		return
	}

	serverURL = arr[0]
	u, err := url.Parse(serverURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats-top: error parsing server url(%s): %v\n", serverURL, err)
		return
	}

	return u.Hostname()
}
