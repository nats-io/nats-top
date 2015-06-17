// Copyright (c) 2015 NATS Messaging System
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func request(path string, opts map[string]interface{}) (map[string]interface{}, error) {
	var statz map[string]interface{}
	uri := fmt.Sprintf("http://%s:%d%s", opts["host"], opts["port"], path)

	if path == "/connz" {
		uri += fmt.Sprintf("?n=%d&s=%s", opts["conns"], opts["sort"])
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

func usage() {
	log.Fatalf("Usage: nats-top [-s server] [-m monitor] [-n num_connections] [-d delay_secs]\n")
}

func psize(size float64) string {
	if size < 1024 {
		return fmt.Sprintf("%.1f", size)
	} else if size < (1024 * 1024) {
		return fmt.Sprintf("%.1fK", size/1024)
	} else if size < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.1fM", size/1024/1024)
	} else if size > (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.1fG", size/1024/1024/1024)
	} else {
		return "NA"
	}
}

var (
	host  = flag.String("s", "127.0.0.1", "The nats server host")
	port  = flag.Int("m", 8333, "The nats server monitoring port")
	conns = flag.Int("n", 1024, "Num of connections")
	delay = flag.Int("d", 1, "Delay in monitoring interval in seconds")
	sort  = flag.String("sort", "pending_size", "Value for which to sort connections")
)

func init() {
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()
}

func main() {
	opts := make(map[string]interface{})
	opts["host"] = *host
	opts["port"] = *port
	opts["conns"] = *conns
	opts["delay"] = *delay
	opts["sort"] = *sort

	if opts["host"] == nil || opts["port"] == nil {
		log.Fatalf("Please specify the monitoring port for NATS: -m PORT")
		usage()
	}

	sigch := make(chan os.Signal)
	signal.Notify(sigch, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go StartSimpleUI(opts)

	select {
	case <-sigch:
		clearScreen()
		os.Exit(1)
	}
}

func clearScreen() {
	fmt.Print("\033[2J\033[1;1H")
}

func StartSimpleUI(opts map[string]interface{}) {
	var pollTime time.Time

	var inMsgsDelta float64
	var outMsgsDelta float64
	var inBytesDelta float64
	var outBytesDelta float64

	inMsgsLastVal := 0.0
	outMsgsLastVal := 0.0
	inBytesLastVal := 0.0
	outBytesLastVal := 0.0

	inMsgsRate := 0.0
	outMsgsRate := 0.0
	inBytesRate := 0.0
	outBytesRate := 0.0

	first := true
	pollTime = time.Now()
	for {
		wg := &sync.WaitGroup{}
		wg.Add(2)

		// Periodically poll for the varz, connz and routez
		var varz map[string]interface{}
		go func() {
			var err error
			defer wg.Done()

			varz, err = request("/varz", opts)
			if err != nil {
				log.Fatalf("Failed during varz processing: %v", err)
			}
		}()

		var connz map[string]interface{}
		go func() {
			var err error
			defer wg.Done()

			connz, err = request("/connz", opts)
			if err != nil {
				log.Fatalf("Failed during connz processing: %v", err)
			}
		}()
		wg.Wait()

		cpu := varz["cpu"].(float64)
		numConns := connz["num_connections"].(float64)
		memVal := varz["mem"].(float64)

		// Periodic snapshot to get per sec metrics
		inMsgsVal := varz["in_msgs"].(float64)
		outMsgsVal := varz["out_msgs"].(float64)
		inBytesVal := varz["in_bytes"].(float64)
		outBytesVal := varz["out_bytes"].(float64)

		inMsgsDelta = inMsgsVal - inMsgsLastVal
		outMsgsDelta = outMsgsVal - outMsgsLastVal
		inBytesDelta = inBytesVal - inBytesLastVal
		outBytesDelta = outBytesVal - outBytesLastVal

		inMsgsLastVal = inMsgsVal
		outMsgsLastVal = outMsgsVal
		inBytesLastVal = inBytesVal
		outBytesLastVal = outBytesVal

		now := time.Now()
		tdelta := pollTime.Sub(now)
		pollTime = now

		// Calculate rates but the first time
		if !first {
			inMsgsRate = inMsgsDelta - tdelta.Seconds()
			outMsgsRate = outMsgsDelta - tdelta.Seconds()
			inBytesRate = inBytesDelta - tdelta.Seconds()
			outBytesRate = outBytesDelta - tdelta.Seconds()
		}

		mem := psize(memVal)
		inMsgs := psize(inMsgsVal)
		outMsgs := psize(outMsgsVal)
		inBytes := psize(inBytesVal)
		outBytes := psize(outBytesVal)

		info := "\nServer:\n  Load: CPU: %.1f%% Memory: %s\n"
		info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f\n"
		info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f"

		text := fmt.Sprintf(info, cpu, mem,
			inMsgs, inBytes, inMsgsRate, inBytesRate,
			outMsgs, outBytes, outMsgsRate, outBytesRate)
		text += fmt.Sprintf("\n\nConnections: %.0f\n", numConns)

		connHeader := "  %-20s %-8s %-6s  %-10s  %-10s  %-10s  %-10s  %-10s\n"
		connRows := fmt.Sprintf(connHeader, "HOST", "CID", "SUBS", "PENDING", "MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM")
		text += connRows

		connValues := "  %-20s %-8.0f %-6.0f  %-10.0f  %-10s  %-10s  %-10s  %-10s\n"
		conns := connz["connections"].([]interface{})
		for _, conn := range conns {
			c := conn.(map[string]interface{})
			host := fmt.Sprintf("%s:%.0f", c["ip"], c["port"])
			connOutMsgs := c["out_msgs"].(float64)
			connInMsgs := c["in_msgs"].(float64)
			connOutBytes := c["out_bytes"].(float64)
			connInBytes := c["in_bytes"].(float64)
			connLine := fmt.Sprintf(connValues, host, c["cid"], c["subscriptions"], c["pending_size"],
				psize(connOutMsgs), psize(connInMsgs), psize(connOutBytes), psize(connInBytes))
			text += connLine
		}
		fmt.Print(text)

		if first {
			first = false
		}

		if val, ok := opts["delay"].(int); ok {
			time.Sleep(time.Duration(val) * time.Second)
			clearScreen()
		} else {
			log.Fatalf("error: could not use %s as a refreshing interval", opts["delay"])
			break
		}
	}
}
