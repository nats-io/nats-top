// Copyright (c) 2015-2022 The NATS Authors
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	top "github.com/nats-io/nats-top/util"
	ui "gopkg.in/gizak/termui.v1"
)

const version = "0.5.0"

var (
	host              = flag.String("s", "127.0.0.1", "The nats server host.")
	port              = flag.Int("m", 8222, "The NATS server monitoring port.")
	conns             = flag.Int("n", 1024, "Maximum number of connections to poll.")
	delay             = flag.Int("d", 1, "Refresh interval in seconds.")
	sortBy            = flag.String("sort", "cid", "Value for which to sort by the connections.")
	lookupDNS         = flag.Bool("lookup", false, "Enable client addresses DNS lookup.")
	outputFile        = flag.String("o", "", "Save the very first nats-top snapshot to the given file and exit. If '-' is passed then the snapshot is printed the standard output.")
	showVersion       = flag.Bool("v", false, "Show nats-top version.")
	displayRawBytes   = flag.Bool("b", false, "Display traffic in raw bytes.")
	maxStatsRefreshes = flag.Int("r", -1, "Specifies the maximum number of times nats-top should refresh nats-stats before exiting.")

	// Secure options
	httpsPort     = flag.Int("ms", 0, "The NATS server secure monitoring port.")
	certOpt       = flag.String("cert", "", "Client cert in case NATS server using TLS")
	keyOpt        = flag.String("key", "", "Client private key in case NATS server using TLS")
	caCertOpt     = flag.String("cacert", "", "Root CA cert")
	skipVerifyOpt = flag.Bool("k", false, "Skip verifying server certificate")
)

const (
	DEFAULT_PADDING_SIZE = 2
	DEFAULT_PADDING      = "  "

	DEFAULT_HOST_PADDING_SIZE = 15
)

var (
	// Chopped: HOST CID NAME...
	defaultHeaderFormat = "%-6s  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s  %-7s  %-40s"
	defaultRowFormat    = "%-6d  %-10s  %-10s  %-10s  %-10s  %-10s  %-7s  %-7s  %-7s  %-40s"

	usageHelp = `
usage: nats-top [-s server] [-m http_port] [-ms https_port] [-n num_connections] [-d delay_secs] [-r max] [-sort by]
                [-cert FILE] [-key FILE ][-cacert FILE] [-k] [-b]

`
	// cache for reducing DNS lookups in case enabled
	resolvedHosts = map[string]string{}
)

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
			fmt.Fprintf(os.Stderr, "nats-top: %s", err)
			usage()
		}
	} else {
		engine = top.NewEngine(*host, *port, *conns, *delay)
		engine.SetupHTTP()
	}

	if engine.Host == "" {
		fmt.Fprintf(os.Stderr, "nats-top: invalid monitoring endpoint")
		usage()
	}

	if engine.Port == 0 {
		fmt.Fprintf(os.Stderr, "nats-top: invalid monitoring port")
		usage()
	}

	// Smoke test to abort in case can't connect to server since the beginning.
	_, err := engine.Request("/varz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats-top: /varz smoke test failed: %s", err)
		usage()
	}

	sortOpt := server.SortOpt(*sortBy)
	if !sortOpt.IsValid() {
		fmt.Fprintf(os.Stderr, "nats-top: invalid option to sort by: %s\n", sortOpt)
		usage()
	}
	engine.SortOpt = sortOpt

	if *outputFile != "" {
		saveStatsSnapshotToFile(engine, outputFile)
		return
	}

	err = ui.Init()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	go engine.MonitorStats()

	StartUI(engine)
}

func saveStatsSnapshotToFile(engine *top.Engine, outputFile *string) {
	stats := engine.FetchStatsSnapshot()
	text := generateParagraph(engine, stats)

	if *outputFile == "-" {
		fmt.Print(text)
		return
	}

	f, err := os.OpenFile(*outputFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalf("nats-top: failed to open output file '%s': %s\n", *outputFile, err)
	}

	if _, err = f.WriteString(text); err != nil {
		log.Fatalf("nats-top: failed to write stats-snapshot to output file '%s': %s\n", *outputFile, err)
	}

	f.Close() //no point to error check    process will exit anyway
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
	if stats.Varz.Version != "" {
		serverVersion = stats.Varz.Version
	}

	mem := top.Psize(false, memVal) //memory is exempt from the rawbytes flag
	inMsgs := top.Psize(*displayRawBytes, inMsgsVal)
	outMsgs := top.Psize(*displayRawBytes, outMsgsVal)
	inBytes := top.Psize(*displayRawBytes, inBytesVal)
	outBytes := top.Psize(*displayRawBytes, outBytesVal)
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.InBytesRate))
	outBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.OutBytesRate))

	info := "NATS server version %s (uptime: %s) %s"
	info += "\nServer:\n  Load: CPU:  %.1f%%  Memory: %s  Slow Consumers: %d\n"
	info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s\n"
	info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s"

	text := fmt.Sprintf(info, serverVersion, uptime, stats.Error,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate)
	text += fmt.Sprintf("\n\nConnections Polled: %d\n", numConns)
	displaySubs := engine.DisplaySubs

	// Dynamically add columns and padding depending
	header := make([]interface{}, 0)
	hostSize := DEFAULT_HOST_PADDING_SIZE

	// Disable name unless we have seen one using it
	nameSize := 0
	for _, conn := range stats.Connz.Conns {
		var size int

		var hostname string
		if *lookupDNS {
			// Make a lookup for each one of the ips and memoize
			// them for subsequent polls.
			if addr, present := resolvedHosts[conn.IP]; !present {
				addrs, err := net.LookupAddr(conn.IP)
				if err == nil && len(addrs) > 0 && len(addrs[0]) > 0 {
					hostname = addrs[0]
					resolvedHosts[conn.IP] = hostname
				} else {
					// Otherwise just continue to use ip:port as resolved host
					// can be an empty string even though there were no errors.
					hostname = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
					resolvedHosts[conn.IP] = hostname
				}
			} else {
				hostname = addr
			}
		} else {
			hostname = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
		}

		// host
		size = len(hostname)
		if size > hostSize {
			hostSize = size + DEFAULT_PADDING_SIZE
		}

		// name
		size = len(conn.Name)
		if size > nameSize {
			nameSize = size + DEFAULT_PADDING_SIZE

			// If using name, ensure that it is not too small...
			minLen := len("NAME")
			if nameSize < minLen {
				nameSize = minLen
			}
		}
	}

	// Initial padding
	connHeader := DEFAULT_PADDING

	// HOST
	header = append(header, "HOST")
	connHeader += "%-" + fmt.Sprintf("%d", hostSize) + "s "

	// CID
	header = append(header, "CID")
	connHeader += " %-6s "

	// NAME
	if nameSize > 0 {
		header = append(header, "NAME")
		connHeader += "%-" + fmt.Sprintf("%d", nameSize) + "s "
	}

	header = append(header, "SUBS", "PENDING", "MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM", "LANG", "VERSION", "UPTIME", "LAST ACTIVITY")
	connHeader += defaultHeaderFormat
	if displaySubs {
		connHeader += "%13s"
	}
	// ...LAST ACTIVITY
	connHeader += "\n"

	var connRows string
	if displaySubs {
		header = append(header, "SUBSCRIPTIONS")
		connRows = fmt.Sprintf(connHeader, header...)
	} else {
		connRows = fmt.Sprintf(connHeader, header...)
	}

	// Add to screen!
	text += connRows

	connValues := DEFAULT_PADDING

	// HOST: e.g. 192.168.1.1:78901
	connValues += "%-" + fmt.Sprintf("%d", hostSize) + "s "

	// CID: e.g. 1234
	connValues += " %-6d "

	// NAME: e.g. hello
	if nameSize > 0 {
		connValues += "%-" + fmt.Sprintf("%d", nameSize) + "s "
	}

	connValues += defaultRowFormat
	if displaySubs {
		connValues += "%s"
	}
	connValues += "\n"

	for _, conn := range stats.Connz.Conns {
		var h string
		if *lookupDNS {
			if rh, present := resolvedHosts[conn.IP]; present {
				h = rh
			}
		} else {
			h = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
		}

		// Build the info line
		var connLine string
		connLineInfo := make([]interface{}, 0)
		connLineInfo = append(connLineInfo, h)
		connLineInfo = append(connLineInfo, conn.Cid)

		// Name not included unless present
		if nameSize > 0 {
			connLineInfo = append(connLineInfo, conn.Name)
		}

		connLineInfo = append(connLineInfo, conn.NumSubs)
		connLineInfo = append(connLineInfo, top.Psize(*displayRawBytes, int64(conn.Pending)), top.Psize(*displayRawBytes, conn.OutMsgs), top.Psize(*displayRawBytes, conn.InMsgs))
		connLineInfo = append(connLineInfo, top.Psize(*displayRawBytes, conn.OutBytes), top.Psize(*displayRawBytes, conn.InBytes))
		connLineInfo = append(connLineInfo, conn.Lang, conn.Version)
		connLineInfo = append(connLineInfo, conn.Uptime, conn.LastActivity)

		if displaySubs {
			subs := strings.Join(conn.Subs, ", ")
			connLineInfo = append(connLineInfo, subs)
			connLine = fmt.Sprintf(connValues, connLineInfo...)
		} else {
			connLine = fmt.Sprintf(connValues, connLineInfo...)
		}

		// Add line to screen!
		text += connLine
	}

	return text
}

type ViewMode int

const (
	TopViewMode ViewMode = iota
	HelpViewMode
)

type RedrawCause int

const (
	DueToNewStats RedrawCause = iota
	DueToViewportResize
)

// StartUI periodically refreshes the screen using recent data.
func StartUI(engine *top.Engine) {

	cleanStats := &top.Stats{
		Varz:  &server.Varz{},
		Connz: &server.Connz{},
		Rates: &top.Rates{},
		Error: fmt.Errorf(""),
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
	redraw := make(chan RedrawCause)

	update := func() {
		for {
			stats := <-engine.StatsCh

			par.Text = generateParagraph(engine, stats) // Update top view text

			redraw <- DueToNewStats
		}
	}

	// Flags for capturing options
	waitingSortOption := false
	waitingLimitOption := false
	displaySubscriptions := false

	optionBuf := ""
	refreshOptionHeader := func() {
		clrline := "\033[1;1H\033[6;1H                  " // Need to mask what was typed before

		clrline += "  "
		for i := 0; i < len(optionBuf); i++ {
			clrline += "  "
		}
		fmt.Print(clrline)
	}

	evt := ui.EventCh()

	ui.Render(ui.Body)

	go update()

	numberOfRedrawsDueToNewStats := 0
	for {
		select {
		case e := <-evt:

			if waitingSortOption {

				if e.Type == ui.EventKey && e.Key == ui.KeyEnter {

					sortOpt := server.SortOpt(optionBuf)
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

			if e.Type == ui.EventKey && (e.Ch == 'd') && !(waitingSortOption || waitingLimitOption) {
				switch *lookupDNS {
				case true:
					*lookupDNS = false
				case false:
					*lookupDNS = true
				}
			}

			if e.Type == ui.EventKey && (e.Ch == 'b') && !(waitingSortOption || waitingLimitOption) {
				switch *displayRawBytes {
				case true:
					*displayRawBytes = false
				case false:
					*displayRawBytes = true
				}
			}

			if e.Type == ui.EventResize {
				ui.Body.Width = ui.TermWidth()
				ui.Body.Align()
				go func() { redraw <- DueToViewportResize }()
			}

		case cause := <-redraw:
			ui.Render(ui.Body)

			if cause == DueToNewStats {
				numberOfRedrawsDueToNewStats += 1

				if *maxStatsRefreshes > 0 && numberOfRedrawsDueToNewStats >= *maxStatsRefreshes {
					close(engine.ShutdownCh)
					cleanExit()
				}
			}
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

d                Toggle activating DNS address lookup for clients.

b                Toggle displaying raw bytes.

q                Quit nats-top.

Press any key to continue...

`
	return text
}
