// Copyright (c) 2015 NATS Messaging System
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	gnatsd "github.com/nats-io/gnatsd/server"
	top "github.com/nats-io/nats-top/util"
	ui "gopkg.in/gizak/termui.v1"
)

const version = "0.2.0"

var (
	host        = flag.String("s", "127.0.0.1", "The nats server host.")
	port        = flag.Int("m", 8222, "The NATS server monitoring port.")
	conns       = flag.Int("n", 1024, "Maximum number of connections to poll.")
	delay       = flag.Int("d", 1, "Refresh interval in seconds.")
	sortBy      = flag.String("sort", "cid", "Value for which to sort by the connections.")
	showVersion = flag.Bool("v", false, "Show nats-top version.")

	// Secure options
	httpsPort     = flag.Int("ms", 0, "The NATS server secure monitoring port.")
	certOpt       = flag.String("cert", "", "Client cert in case NATS server using TLS")
	keyOpt        = flag.String("key", "", "Client private key in case NATS server using TLS")
	caCertOpt     = flag.String("cacert", "", "Root CA cert")
	skipVerifyOpt = flag.Bool("k", false, "Skip verifying server certificate")
)

var usageHelp = `
usage: nats-top [-s server] [-m http_port] [-ms https_port] [-n num_connections] [-d delay_secs] [-sort by]
                [-cert FILE] [-key FILE ][-cacert FILE] [-k]

`

func usage() {
	log.Fatalf(usageHelp)
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

	var engine *top.Engine

	// Use secure port if set explicitly, otherwise use http port by default
	if *httpsPort != 0 {
		engine = top.NewEngine(*host, *httpsPort, *conns, *delay)
		err := engine.SetupHTTPS(*caCertOpt, *certOpt, *keyOpt, *skipVerifyOpt)
		if err != nil {
			log.Printf("nats-top: %s", err)
			usage()
		}
	} else {
		engine = top.NewEngine(*host, *port, *conns, *delay)
		engine.SetupHTTP()
	}

	if engine.Host == "" {
		log.Printf("nats-top: invalid monitoring endpoint")
		usage()
	}

	if engine.Port == 0 {
		log.Printf("nats-top: invalid monitoring port")
		usage()
	}

	// Smoke test to abort in case can't connect to server since the beginning.
	_, err := engine.Request("/varz")
	if err != nil {
		log.Printf("nats-top: %s", err)
		usage()
	}

	sortOpt := gnatsd.SortOpt(*sortBy)
	if !sortOpt.IsValid() {
		log.Fatalf("nats-top: invalid option to sort by: %s\n", sortOpt)
		usage()
	}
	engine.SortOpt = sortOpt

	err = ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	go engine.MonitorStats()
	StartUI(engine)
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
	engine *top.Engine,
	stats *top.Stats,
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

	mem := top.Psize(memVal)
	inMsgs := top.Psize(inMsgsVal)
	outMsgs := top.Psize(outMsgsVal)
	inBytes := top.Psize(inBytesVal)
	outBytes := top.Psize(outBytesVal)
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := top.Psize(int64(stats.Rates.InBytesRate))
	outBytesRate := top.Psize(int64(stats.Rates.OutBytesRate))

	info := "NATS server version %s (uptime: %s)"
	info += "\nServer:\n  Load: CPU:  %.1f%%  Memory: %s  Slow Consumers: %d\n"
	info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s\n"
	info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s"

	text := fmt.Sprintf(info, serverVersion, uptime,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate)
	text += fmt.Sprintf("\n\nConnections: %d\n", numConns)
	displaySubs := engine.DisplaySubs

	connHeader := "  %-20s %-8s %-15s %-6s  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s  %-7s  %-40s"
	if displaySubs {
		connHeader += "%13s"
	}
	connHeader += "\n"

	var connRows string
	var connValues string
	if displaySubs {
		connRows = fmt.Sprintf(connHeader, "HOST", "CID", "NAME", "SUBS", "PENDING",
			"MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
			"LANG", "VERSION", "UPTIME", "LAST ACTIVITY", "SUBSCRIPTIONS")
	} else {
		connRows = fmt.Sprintf(connHeader, "HOST", "CID", "NAME", "SUBS", "PENDING",
			"MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM",
			"LANG", "VERSION", "UPTIME", "LAST ACTIVITY")
	}
	text += connRows

	connValues = "  %-20s %-8d %-15s %-6d  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s  %-7s  %-40s"
	if displaySubs {
		connValues += "%s"
	}
	connValues += "\n"

	for _, conn := range stats.Connz.Conns {
		host := fmt.Sprintf("%s:%d", conn.IP, conn.Port)

		var connLine string
		if displaySubs {
			subs := strings.Join(conn.Subs, ", ")
			connLine = fmt.Sprintf(connValues, host, conn.Cid, conn.Name, conn.NumSubs, top.Psize(int64(conn.Pending)),
				top.Psize(conn.OutMsgs), top.Psize(conn.InMsgs), top.Psize(conn.OutBytes), top.Psize(conn.InBytes),
				conn.Lang, conn.Version, conn.Uptime, conn.LastActivity, subs)
		} else {
			connLine = fmt.Sprintf(connValues, host, conn.Cid, conn.Name, conn.NumSubs, top.Psize(int64(conn.Pending)),
				top.Psize(conn.OutMsgs), top.Psize(conn.InMsgs), top.Psize(conn.OutBytes), top.Psize(conn.InBytes),
				conn.Lang, conn.Version, conn.Uptime, conn.LastActivity)
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

// StartUI periodically refreshes the screen using recent data.
func StartUI(engine *top.Engine) {

	cleanStats := &top.Stats{
		Varz:  &gnatsd.Varz{},
		Connz: &gnatsd.Connz{},
		Rates: &top.Rates{},
	}

	// Show empty values on first display
	text := generateParagraph(engine, cleanStats)
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
			stats := <-engine.StatsCh

			// Update top view text
			text = generateParagraph(engine, stats)
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
					if sortOpt.IsValid() {
						engine.SortOpt = sortOpt
					} else {
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
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]: %s", engine.SortOpt, optionBuf)
			}

			if waitingLimitOption {

				if e.Type == ui.EventKey && e.Key == ui.KeyEnter {

					var n int
					_, err := fmt.Sscanf(optionBuf, "%d", &n)
					if err == nil {
						engine.Conns = n
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
				fmt.Printf("\033[1;1H\033[6;1Hlimit   [%d]: %s", engine.Conns, optionBuf)
			}

			if e.Type == ui.EventKey && (e.Ch == 'q' || e.Key == ui.KeyCtrlC) {
				close(engine.ShutdownCh)
				cleanExit()
			}

			if e.Type == ui.EventKey && e.Ch == 's' && !(waitingLimitOption || waitingSortOption) {
				if displaySubscriptions {
					displaySubscriptions = false
					engine.DisplaySubs = false
				} else {
					displaySubscriptions = true
					engine.DisplaySubs = true
				}
			}

			if e.Type == ui.EventKey && viewMode == HelpViewMode {
				ui.Body.Rows = topViewGrid.Rows
				viewMode = TopViewMode
				continue
			}

			if e.Type == ui.EventKey && e.Ch == 'o' && !waitingLimitOption && viewMode == TopViewMode {
				fmt.Printf("\033[1;1H\033[6;1Hsort by [%s]:", engine.SortOpt)
				waitingSortOption = true
			}

			if e.Type == ui.EventKey && e.Ch == 'n' && !waitingSortOption && viewMode == TopViewMode {
				fmt.Printf("\033[1;1H\033[6;1Hlimit   [%d]:", engine.Conns)
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

                 Option can be one of: {cid|subs|pending|msgs_to|msgs_from|
                 bytes_to|bytes_from|idle|last}

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
