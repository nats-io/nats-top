// Copyright (c) 2015 NATS Messaging System
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/gnatsd/server"
	. "github.com/nats-io/nats-top/util"
)

func usage() {
	log.Fatalf("Usage: nats-top [-s server] [-m monitor] [-n num_connections] [-d delay_secs]\n")
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
		log.Fatalf("Please specify the monitoring port for NATS.")
		usage()
	}

	// Smoke test the server once before starting
	_, err := Request("/varz", opts)
	if err != nil {
		log.Fatalf("ERROR: %v", err)
		os.Exit(1)
	}

	sigch := make(chan os.Signal)
	signal.Notify(sigch, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	clearScreen()
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
	for {
		wg := &sync.WaitGroup{}
		wg.Add(2)

		// Periodically poll for the varz, connz and routez
		var varz *server.Varz
		go func() {
			var err error
			defer wg.Done()

			result, err := Request("/varz", opts)
			if err != nil {
				log.Fatalf("Could not get /varz: %v", err)
			}

			if varzVal, ok := result.(*server.Varz); ok {
				varz = varzVal
			}
		}()

		var connz *server.Connz
		go func() {
			var err error
			defer wg.Done()

			result, err := Request("/connz", opts)
			if err != nil {
				log.Fatalf("Could not get /connz: %v", err)
			}

			if connzVal, ok := result.(*server.Connz); ok {
				connz = connzVal
			}
		}()
		wg.Wait()

		cpu := varz.CPU
		numConns := connz.NumConns
		memVal := varz.Mem

		// Periodic snapshot to get per sec metrics
		inMsgsVal := varz.InMsgs
		outMsgsVal := varz.OutMsgs
		inBytesVal := varz.InBytes
		outBytesVal := varz.OutBytes

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
			inMsgsRate = float64(inMsgsDelta) - tdelta.Seconds()
			outMsgsRate = float64(outMsgsDelta) - tdelta.Seconds()
			inBytesRate = float64(inBytesDelta) - tdelta.Seconds()
			outBytesRate = float64(outBytesDelta) - tdelta.Seconds()
		}

		mem := Psize(memVal)
		inMsgs := Psize(inMsgsVal)
		outMsgs := Psize(outMsgsVal)
		inBytes := Psize(inBytesVal)
		outBytes := Psize(outBytesVal)

		info := "\nServer:\n  Load: CPU: %.1f%%  Memory: %s\n"
		info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f\n"
		info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f"

		text := fmt.Sprintf(info, cpu, mem,
			inMsgs, inBytes, inMsgsRate, inBytesRate,
			outMsgs, outBytes, outMsgsRate, outBytesRate)
		text += fmt.Sprintf("\n\nConnections: %d\n", numConns)

		connHeader := "  %-20s %-8s %-6s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s\n"
		connRows := fmt.Sprintf(connHeader, "HOST", "CID", "SUBS", "PENDING",
			                            "MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
			                            "LANG", "VERSION")
		text += connRows

		connValues := "  %-20s %-8d %-6d  %-10d  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s\n"
		for _, conn := range connz.Conns {
			host := fmt.Sprintf("%s:%d", conn.IP, conn.Port)
			connLine := fmt.Sprintf(connValues, host, conn.Cid, conn.NumSubs, conn.Pending,
				Psize(conn.OutMsgs), Psize(conn.InMsgs), Psize(conn.OutBytes), Psize(conn.InBytes),
				conn.Lang, conn.Version)
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
