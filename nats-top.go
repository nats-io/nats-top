// Copyright (c) 2015 NATS Messaging System
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
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
	showVersion = flag.Bool("v", false, "Show nats-top version.")
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
		usage()
	}

	err := ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	statsCh := make(chan *Stats)
	shutdownCh := make(chan struct{})

	go MonitorStats(opts, statsCh, shutdownCh)

	StartUI(opts, statsCh, shutdownCh)
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

// generateParagraph takes an options map and latest Stats
// then returns a formatted paragraph ready to be rendered
func generateParagraph(
	opts map[string]interface{},
	stats *Stats,
) string {

	// Snapshot current stats
	cpu := stats.Varz.CPU
	memVal := stats.Varz.Mem
	uptime := stats.Varz.Uptime
	numConns := stats.Connz.NumConns
	inMsgsVal := stats.Varz.InMsgs
	outMsgsVal := stats.Varz.OutMsgs
	inBytesVal := stats.Varz.InBytes
	outBytesVal := stats.Varz.OutBytes
	slowConsumers := stats.Varz.SlowConsumers

	var serverVersion string
	if stats.Varz.Info != nil {
		serverVersion = stats.Varz.Info.Version
	}

	mem := Psize(memVal)
	inMsgs := Psize(inMsgsVal)
	outMsgs := Psize(outMsgsVal)
	inBytes := Psize(inBytesVal)
	outBytes := Psize(outBytesVal)
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := Psize(int64(stats.Rates.InBytesRate))
	outBytesRate := Psize(int64(stats.Rates.OutBytesRate))

	info := "gnatsd version %s (uptime: %s)"
	info += "\nServer:\n  Load: CPU:  %.1f%%  Memory: %s  Slow Consumers: %d\n"
	info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s\n"
	info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s"

	text := fmt.Sprintf(info, serverVersion, uptime,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate)
	text += fmt.Sprintf("\n\nConnections: %d\n", numConns)

	var displaySubs bool
	if val, ok := opts["subs"]; ok {
		displaySubs = val.(bool)
	}

	connHeader := "  %-20s %-8s %-6s  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s "
	if displaySubs {
		connHeader += "%13s"
	}
	connHeader += "\n"

	var connRows string
	var connValues string
	if displaySubs {
		connRows = fmt.Sprintf(connHeader, "HOST", "CID", "SUBS", "PENDING",
			"MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
			"LANG", "VERSION", "SUBSCRIPTIONS")
	} else {
		connRows = fmt.Sprintf(connHeader, "HOST", "CID", "SUBS", "PENDING",
			"MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
			"LANG", "VERSION")
	}
	text += connRows

	connValues = "  %-20s %-8d %-6d  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s "
	if displaySubs {
		connValues += "%s"
	}
	connValues += "\n"

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

		var connLine string
		if displaySubs {
			subs := strings.Join(conn.Subs, ", ")
			connLine = fmt.Sprintf(connValues, host, conn.Cid, conn.NumSubs, Psize(int64(conn.Pending)),
				Psize(conn.OutMsgs), Psize(conn.InMsgs), Psize(conn.OutBytes), Psize(conn.InBytes),
				conn.Lang, conn.Version, subs)
		} else {
			connLine = fmt.Sprintf(connValues, host, conn.Cid, conn.NumSubs, Psize(int64(conn.Pending)),
				Psize(conn.OutMsgs), Psize(conn.InMsgs), Psize(conn.OutBytes), Psize(conn.InBytes),
				conn.Lang, conn.Version)
		}

		text += connLine
	}

	return text
}

type ViewMode int

const (
	TopViewMode ViewMode = iota
	HelpViewMode
)

// StartUI periodically refreshes the screen using recent data
func StartUI(
	opts map[string]interface{},
	statsCh chan *Stats,
	shutdownCh chan struct{},
) {

	cleanStats := &Stats{
		Varz:  &gnatsd.Varz{},
		Connz: &gnatsd.Connz{},
		Rates: &Rates{},
	}

	// Show empty values on first display
	text := generateParagraph(opts, cleanStats)
	par := ui.NewPar(text)
	par.Height = ui.TermHeight()
	par.Width = ui.TermWidth()
	par.HasBorder = false

	helpText := generateHelp()
	helpPar := ui.NewPar(helpText)
	helpPar.Height = ui.TermHeight()
	helpPar.Width = ui.TermWidth()
	helpPar.HasBorder = false

	// Top like view
	paraRow := ui.NewRow(ui.NewCol(ui.TermWidth(), 0, par))

	// Help view
	helpParaRow := ui.NewRow(ui.NewCol(ui.TermWidth(), 0, helpPar))

	// Create grids that we'll be using to toggle what to render
	topViewGrid := ui.NewGrid(paraRow)
	helpViewGrid := ui.NewGrid(helpParaRow)

	// Start with the topviewGrid by default
	ui.Body.Rows = topViewGrid.Rows
	ui.Body.Align()

	// Used to toggle back to previous mode
	viewMode := TopViewMode

	// Used for pinging the IU to refresh the screen with new values
	redraw := make(chan struct{})

	update := func() {
		for {
			stats := <-statsCh

			// Update top view text
			text = generateParagraph(opts, stats)
			par.Text = text

			redraw <- struct{}{}
		}
	}

	// Flags for capturing options
	waitingSortOption := false
	waitingLimitOption := false
	displaySubscriptions := false

	optionBuf := ""
	refreshOptionHeader := func() {
		// Need to mask what was typed before
		clrline := "\033[1;1H\033[6;1H                  "

		clrline += "  "
		for i := 0; i < len(optionBuf); i++ {
			clrline += "  "
		}
		fmt.Printf(clrline)
	}

	evt := ui.EventCh()

	ui.Render(ui.Body)

	go update()

	for {
		select {
		case e := <-evt:

			if waitingSortOption {

				if e.Type == ui.EventKey && e.Key == ui.KeyEnter {

					sortOpt := gnatsd.SortOpt(optionBuf)
					switch sortOpt {
					case SortByCid, SortBySubs, SortByOutMsgs, SortByInMsgs, SortByOutBytes, SortByInBytes:
						opts["sort"] = sortOpt
					default:
						go func() {
							// Has to be at least of the same length as sort by header
							emptyPadding := "       "
							fmt.Printf("\033[1;1H\033[6;1Hinvalid order: %s%s", optionBuf, emptyPadding)
							waitingSortOption = false
							time.Sleep(1 * time.Second)
							refreshOptionHeader()
							optionBuf = ""
						}()
						continue
					}

					refreshOptionHeader()
					waitingSortOption = false
					optionBuf = ""
					continue
				}

				// Handle backspace
				if e.Type == ui.EventKey && len(optionBuf) > 0 && (e.Key == ui.KeyBackspace || e.Key == ui.KeyBackspace2) {
					optionBuf = optionBuf[:len(optionBuf)-1]
					refreshOptionHeader()
				} else {
					optionBuf += string(e.Ch)
				}
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]: %s", opts["sort"], optionBuf)
			}

			if waitingLimitOption {

				if e.Type == ui.EventKey && e.Key == ui.KeyEnter {

					var n int
					_, err := fmt.Sscanf(optionBuf, "%d", &n)
					if err == nil {
						opts["conns"] = n
					}

					waitingLimitOption = false
					optionBuf = ""
					refreshOptionHeader()
					continue
				}

				// Handle backspace
				if e.Type == ui.EventKey && len(optionBuf) > 0 && (e.Key == ui.KeyBackspace || e.Key == ui.KeyBackspace2) {
					optionBuf = optionBuf[:len(optionBuf)-1]
					refreshOptionHeader()
				} else {
					optionBuf += string(e.Ch)
				}
				fmt.Printf("\033[1;1H\033[6;1Hlimit   [%d]: %s", opts["conns"], optionBuf)
			}

			if e.Type == ui.EventKey && e.Ch == 'q' {
				close(shutdownCh)
				cleanExit()
			}

			if e.Type == ui.EventKey && e.Ch == 's' && !(waitingLimitOption || waitingSortOption) {
				if displaySubscriptions {
					displaySubscriptions = false
					opts["subs"] = false
				} else {
					displaySubscriptions = true
					opts["subs"] = true
				}
			}

			if e.Type == ui.EventKey && viewMode == HelpViewMode {
				ui.Body.Rows = topViewGrid.Rows
				viewMode = TopViewMode
				continue
			}

			if e.Type == ui.EventKey && e.Ch == 'o' && !waitingLimitOption && viewMode == TopViewMode {
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]:", opts["sort"])
				waitingSortOption = true
			}

			if e.Type == ui.EventKey && e.Ch == 'n' && !waitingSortOption && viewMode == TopViewMode {
				fmt.Printf("\033[1;1H\033[6;1Hlimit   [%d]:", opts["conns"])
				waitingLimitOption = true
			}

			if e.Type == ui.EventKey && (e.Ch == '?' || e.Ch == 'h') && !(waitingSortOption || waitingLimitOption) {
				if viewMode == TopViewMode {
					refreshOptionHeader()
					optionBuf = ""
				}

				ui.Body.Rows = helpViewGrid.Rows
				viewMode = HelpViewMode
				waitingLimitOption = false
				waitingSortOption = false
			}

			if e.Type == ui.EventResize {
				ui.Body.Width = ui.TermWidth()
				ui.Body.Align()
				go func() { redraw <- struct{}{} }()
			}

		case <-redraw:
			ui.Render(ui.Body)
		}
	}
}

func generateHelp() string {
	text := `
Command          Description

o<option>        Set primary sort key to <option>.

                 Option can be one of: {cid|subs|msgs_to|msgs_from|
                 bytes_to, bytes_from}

                 This can be set in the command line too with -sort flag.

n<limit>         Set sample size of connections to request from the server.

                 This can be set in the command line as well via -n flag.
                 Note that if used in conjunction with sort, the server 
                 would respect both options allowing queries like 'connection
                 with largest number of subscriptions': -n 1 -sort subs

s                Toggle displaying connection subscriptions.

q                Quit nats-top.

Press any key to continue...

`
	return text
}
