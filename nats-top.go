// Copyright (c) 2015 NATS Messaging System
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	ui "github.com/gizak/termui"
	gnatsd "github.com/nats-io/gnatsd/server"
	. "github.com/nats-io/nats-top/util"
)

const version = "0.1.0"

var (
	host        = flag.String("s", "127.0.0.1", "The nats server host.")
	port        = flag.Int("m", 8222, "The nats server monitoring port.")
	conns       = flag.Int("n", 1024, "Maximum number of connections to poll.")
	delay       = flag.Int("d", 1, "Refresh interval in seconds.")
	sortBy      = flag.String("sort", "cid", "Value for which to sort by the connections.")
	showVersion = flag.Bool("v", false, "Show nats-top version")
)

func usage() {
	log.Fatalf("Usage: nats-top [-s server] [-m monitor_port] [-n num_connections] [-d delay_secs] [-sort by]\n")
}

func init() {
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()
}

func main() {

	if *showVersion {
		log.Printf("nats-top v%s", version)
		os.Exit(0)
	}

	opts := map[string]interface{}{}
	opts["host"] = *host
	opts["port"] = *port
	opts["conns"] = *conns
	opts["delay"] = *delay

	if opts["host"] == nil || opts["port"] == nil {
		log.Fatalf("Please specify the monitoring port for NATS.\n")
		usage()
	}

	sortOpt := gnatsd.SortOpt(*sortBy)
	switch sortOpt {
	case SortByCid, SortBySubs, SortByOutMsgs, SortByInMsgs, SortByOutBytes, SortByInBytes:
		opts["sort"] = sortOpt
	default:
		log.Printf("nats-top: not a valid option to sort by: %s\n", sortOpt)
	}

	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	statsCh := make(chan *Stats)

	go monitorStats(opts, statsCh)

	StartRatesUI(opts, statsCh)
}

// clearScreen tries to ensure resetting original state of screen
func clearScreen() {
	fmt.Print("\033[2J\033[1;1H\033[?25l")
}

func cleanExit() {
	clearScreen()
	ui.Close()

	// Show cursor once again
	fmt.Print("\033[?25h")
	os.Exit(0)
}

func exitWithError() {
	ui.Close()
	os.Exit(1)
}

// monitorStats can be ran as a goroutine and takes options
// which can modify how to do the polling
func monitorStats(
	opts map[string]interface{},
	statsCh chan *Stats,
) {
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
		// Note that delay defines the sampling rate as well
		if val, ok := opts["delay"].(int); ok {
			time.Sleep(time.Duration(val) * time.Second)
		} else {
			log.Fatalf("error: could not use %s as a refreshing interval", opts["delay"])
			break
		}

		// Wrap collected info in a Stats struct
		stats := &Stats{
			Varz:  &gnatsd.Varz{},
			Connz: &gnatsd.Connz{},
			Rates: &Rates{},
		}

		// Get /varz
		{
			result, err := Request("/varz", opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not get /varz: %v", err)
				statsCh <- stats
				continue
			}
			if varz, ok := result.(*gnatsd.Varz); ok {
				stats.Varz = varz
			}
		}

		// Get /connz
		{
			result, err := Request("/connz", opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not get /connz: %v", err)
				statsCh <- stats
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

		// Send update
		statsCh <- stats
	}
}

// generateParagraph takes an options map and latest Stats
// then returns a formatted paragraph ready to be rendered
func generateParagraph(
	opts map[string]interface{},
	stats *Stats,
) string {

	cpu := stats.Varz.CPU
	memVal := stats.Varz.Mem
	uptime := stats.Varz.Uptime
	numConns := stats.Connz.NumConns
	inMsgsVal := stats.Varz.InMsgs
	outMsgsVal := stats.Varz.OutMsgs
	inBytesVal := stats.Varz.InBytes
	outBytesVal := stats.Varz.OutBytes
	slowConsumers := stats.Varz.SlowConsumers
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := stats.Rates.InBytesRate
	outBytesRate := stats.Rates.OutBytesRate

	var serverVersion string
	if stats.Varz.Info != nil {
		serverVersion = stats.Varz.Info.Version
	}

	mem := Psize(memVal)
	inMsgs := Psize(inMsgsVal)
	outMsgs := Psize(outMsgsVal)
	inBytes := Psize(inBytesVal)
	outBytes := Psize(outBytesVal)

	info := "gnatsd version %s (uptime: %s)"
	info += "\nServer:\n  Load: CPU: %.1f%%   Memory: %s   Slow Consumers: %d\n"
	info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f\n"
	info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %.1f"

	text := fmt.Sprintf(info, serverVersion, uptime,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate)
	text += fmt.Sprintf("\n\nConnections: %d\n", numConns)

	connHeader := "  %-20s %-8s %-6s  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s\n"

	connRows := fmt.Sprintf(connHeader, "HOST", "CID", "SUBS", "PENDING",
		"MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
		"LANG", "VERSION")
	text += connRows
	connValues := "  %-20s %-8d %-6d  %-10d  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s\n"

	switch opts["sort"] {
	case SortByCid:
		sort.Sort(ByCid(stats.Connz.Conns))
	case SortBySubs:
		sort.Sort(sort.Reverse(BySubs(stats.Connz.Conns)))
	case SortByOutMsgs:
		sort.Sort(sort.Reverse(ByMsgsTo(stats.Connz.Conns)))
	case SortByInMsgs:
		sort.Sort(sort.Reverse(ByMsgsFrom(stats.Connz.Conns)))
	case SortByOutBytes:
		sort.Sort(sort.Reverse(ByBytesTo(stats.Connz.Conns)))
	case SortByInBytes:
		sort.Sort(sort.Reverse(ByBytesFrom(stats.Connz.Conns)))
	}

	for _, conn := range stats.Connz.Conns {
		host := fmt.Sprintf("%s:%d", conn.IP, conn.Port)
		connLine := fmt.Sprintf(connValues, host, conn.Cid, conn.NumSubs, conn.Pending,
			Psize(conn.OutMsgs), Psize(conn.InMsgs), Psize(conn.OutBytes), Psize(conn.InBytes),
			conn.Lang, conn.Version)
		text += connLine
	}

	return text
}

// StartRatesUI periodically refreshes the state of the screen
func StartRatesUI(
	opts map[string]interface{},
	statsCh chan *Stats,
) {

	cleanStats := &Stats{
		Varz:  &gnatsd.Varz{},
		Connz: &gnatsd.Connz{},
		Rates: &Rates{},
	}
	text := generateParagraph(opts, cleanStats)
	par := ui.NewPar(text)
	par.Height = ui.TermHeight()
	par.Width = ui.TermWidth()
	par.HasBorder = false

	done := make(chan struct{})
	redraw := make(chan bool)

	update := func() {
		for {
			stats := <-statsCh
			text = generateParagraph(opts, stats)
			par.Text = text

			redraw <- true
		}
		done <- struct{}{}
	}

	waitingSortOption := false
	sortOptionBuf := ""
	refreshSortHeader := func() {
		// Need to mask what was typed before
		clrline := "\033[1;1H\033[6;1H                  "
		for i := 0; i < len(opts["sort"].(gnatsd.SortOpt)); i++ {
			clrline += " "
		}
		clrline += "  "
		for i := 0; i < len(sortOptionBuf); i++ {
			clrline += " "
		}
		fmt.Printf(clrline)
	}

	evt := ui.EventCh()
	ui.Render(par)
	go update()

	for {
		select {
		case e := <-evt:

			if waitingSortOption {

				if e.Type == ui.EventKey && e.Key == ui.KeyEnter {

					sortOpt := gnatsd.SortOpt(sortOptionBuf)
					switch sortOpt {
					case SortByCid, SortBySubs, SortByOutMsgs, SortByInMsgs, SortByOutBytes, SortByInBytes:
						opts["sort"] = sortOpt
					default:
						go func() {
							// Has to be at least of the same length as sort by header
							emptyPadding := ""
							if len(sortOptionBuf) < 5 {
								emptyPadding = "     "
							}
							fmt.Printf("\033[1;1H\033[6;1Hinvalid order: %s%s", emptyPadding, sortOptionBuf)
							waitingSortOption = false
							time.Sleep(1 * time.Second)
							refreshSortHeader()
							sortOptionBuf = ""
						}()
						continue
					}

					refreshSortHeader()
					waitingSortOption = false
					sortOptionBuf = ""
					continue
				}

				// Handle backspace
				if e.Type == ui.EventKey && len(sortOptionBuf) > 0 && (e.Key == ui.KeyBackspace || e.Key == ui.KeyBackspace2) {
					sortOptionBuf = sortOptionBuf[:len(sortOptionBuf)-1]
					refreshSortHeader()
				} else {
					sortOptionBuf += string(e.Ch)
				}
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]: %s", opts["sort"], sortOptionBuf)
			}

			if e.Type == ui.EventKey && e.Ch == 'q' {
				cleanExit()
			}

			if e.Type == ui.EventKey && e.Ch == 'o' {
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]:", opts["sort"])
				waitingSortOption = true
			}

			if e.Type == ui.EventKey && e.Key == ui.KeySpace {
				// Not implemented
			}

			if e.Type == ui.EventResize {
				ui.Body.Align()
				go func() { redraw <- true }()
			}

		case <-done:
			return
		case <-redraw:
			ui.Render(par)
		}
	}
}
