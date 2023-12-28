// Copyright (c) 2015-2023 The NATS Authors
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

var (
	host                       = flag.String("s", "127.0.0.1", "The nats server host.")
	port                       = flag.Int("m", 8222, "The NATS server monitoring port.")
	conns                      = flag.Int("n", 1024, "Maximum number of connections to poll.")
	delay                      = flag.Int("d", 1, "Refresh interval in seconds.")
	sortBy                     = flag.String("sort", "cid", "Value for which to sort by the connections.")
	lookupDNS                  = flag.Bool("lookup", false, "Enable client addresses DNS lookup.")
	outputFile                 = flag.String("o", "", "Save the very first nats-top snapshot to the given file and exit. If '-' is passed then the snapshot is printed the standard output.")
	showVersion                = false
	outputDelimiter            = flag.String("l", "", "Specifies the delimiter to use for the output file when the '-o' parameter is used. By default this option is unset which means that standard grid-like plain-text output will be used.")
	displayRawBytes            = flag.Bool("b", false, "Display traffic in raw bytes.")
	maxStatsRefreshes          = flag.Int("r", -1, "Specifies the maximum number of times nats-top should refresh nats-stats before exiting.")
	displaySubscriptionsColumn = false

	// Secure options
	httpsPort     = flag.Int("ms", 0, "The NATS server secure monitoring port.")
	certOpt       = flag.String("cert", "", "Client cert in case NATS server using TLS")
	keyOpt        = flag.String("key", "", "Client private key in case NATS server using TLS")
	caCertOpt     = flag.String("cacert", "", "Root CA cert")
	skipVerifyOpt = flag.Bool("k", false, "Skip verifying server certificate")

	// Nats context options
	contextName = flag.String("context", "", "The name of the context to use. If not specified, the currently selected context is used.")

	version = "0.0.0"
)

const usageHelp = `
usage: nats-top [-s server] [-m http_port] [-ms https_port] [-n num_connections] [-d delay_secs] [-r max] [-o FILE] [-l DELIMITER] [-sort by]
                [-cert FILE] [-key FILE] [-cacert FILE] [-k] [-b] [-v|--version] [-u|--display-subscriptions-column] [--context context_name]

`

func usage() {
	log.Fatalf(usageHelp)
}

func init() {
	flag.BoolVar(&showVersion, "v", false, "Same as --version.")
	flag.BoolVar(&showVersion, "version", false, "Show nats-top version.")

	flag.BoolVar(&displaySubscriptionsColumn, "u", false, "Same as --display-subscriptions-column.")
	flag.BoolVar(&displaySubscriptionsColumn, "display-subscriptions-column", false, "Display subscriptions-column upon launch.")

	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()
}

func main() {
	if showVersion {
		log.Printf("nats-top v%s", version)
		os.Exit(0)
	}

	var engine *top.Engine

	// Use context if set explicitly or if host is not set, otherwise use host
	if *host == "" || *contextName != "" {
		withContext(*contextName)
	}

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

	if displaySubscriptionsColumn {
		engine.DisplaySubs = true
	}

	if *outputFile != "" {
		saveStatsSnapshotToFile(engine, outputFile, *outputDelimiter)
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

func saveStatsSnapshotToFile(engine *top.Engine, outputFile *string, outputDelimiter string) {
	stats := engine.FetchStatsSnapshot()
	text := generateParagraph(engine, stats, outputDelimiter)

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

	f.Close() // no point to error check    process will exit anyway
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
	outputDelimiter string,
) string {

	if len(outputDelimiter) > 0 { // default
		return generateParagraphCSV(engine, stats, outputDelimiter)
	}

	return generateParagraphPlainText(engine, stats)
}

const (
	DEFAULT_PADDING_SIZE      = 2
	DEFAULT_PADDING           = "  "
	DEFAULT_HOST_PADDING_SIZE = 15
	UI_HEADER_PREFIX          = "\033[1;1H\033[7;1H"
)

var (
	resolvedHosts = map[string]string{} // cache for reducing DNS lookups in case enabled

	standardHeaders = []interface{}{"SUBS", "PENDING", "MSGS_TO", "MSGS_FROM", "BYTES_TO", "BYTES_FROM", "LANG", "VERSION", "UPTIME", "LAST_ACTIVITY"}

	defaultHeaderColumns = []string{"%-6s", "%-10s", "%-10s", "%-10s", "%-10s", "%-10s", "%-7s", "%-7s", "%-7s", "%-40s"} // Chopped: HOST CID NAME...
	defaultRowColumns    = []string{"%-6d", "%-10s", "%-10s", "%-10s", "%-10s", "%-10s", "%-7s", "%-7s", "%-7s", "%-40s"}
)

func generateParagraphPlainText(
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
	serverID := stats.Varz.ID

	var serverVersion string
	if stats.Varz.Version != "" {
		serverVersion = stats.Varz.Version
	}
	var serverName string
	if stats.Varz.Name != stats.Varz.ID {
		serverName = stats.Varz.Name
	}

	mem := top.Psize(false, memVal) // memory is exempt from the rawbytes flag
	inMsgs := top.Nsize(*displayRawBytes, inMsgsVal)
	outMsgs := top.Nsize(*displayRawBytes, outMsgsVal)
	inBytes := top.Psize(*displayRawBytes, inBytesVal)
	outBytes := top.Psize(*displayRawBytes, outBytesVal)
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.InBytesRate))
	outBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.OutBytesRate))

	info := "NATS server version %s (uptime: %s) %s\n"
	info += "Server: %s\n"
	info += "  ID:   %s\n"
	info += "  Load: CPU:  %.1f%%  Memory: %s  Slow Consumers: %d\n"
	info += "  In:   Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s\n"
	info += "  Out:  Msgs: %s  Bytes: %s  Msgs/Sec: %.1f  Bytes/Sec: %s"

	text := fmt.Sprintf(
		info, serverVersion, uptime, stats.Error,
		serverName, serverID,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate,
	)

	text += fmt.Sprintf("\n\nConnections Polled: %d\n", numConns)
	displaySubs := engine.DisplaySubs

	header := make([]interface{}, 0) // Dynamically add columns and padding depending
	hostSize := DEFAULT_HOST_PADDING_SIZE

	nameSize := 0 // Disable name unless we have seen one using it
	for _, conn := range stats.Connz.Conns {
		var size int

		var hostname string
		if *lookupDNS {
			if addr, present := resolvedHosts[conn.IP]; !present { // Make a lookup for each one of the ips and memoize them for subsequent polls
				addrs, err := net.LookupAddr(conn.IP)
				if err == nil && len(addrs) > 0 && len(addrs[0]) > 0 {
					hostname = addrs[0]
					resolvedHosts[conn.IP] = hostname
				} else {
					// Otherwise just continue to use ip:port as resolved host
					// can be an empty string even though there were no errors
					hostname = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
					resolvedHosts[conn.IP] = hostname
				}
			} else {
				hostname = addr
			}
		} else {
			hostname = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
		}

		size = len(hostname) // host
		if size > hostSize {
			hostSize = size + DEFAULT_PADDING_SIZE
		}

		size = len(conn.Name) // name
		if size > nameSize {
			nameSize = size + DEFAULT_PADDING_SIZE

			minLen := len("NAME") // If using name, ensure that it is not too small...
			if nameSize < minLen {
				nameSize = minLen
			}
		}
	}

	connHeader := DEFAULT_PADDING // Initial padding

	header = append(header, "HOST") // HOST
	connHeader += "%-" + fmt.Sprintf("%d", hostSize) + "s "

	header = append(header, "CID") // CID
	connHeader += " %-6s "

	if nameSize > 0 { // NAME
		header = append(header, "NAME")
		connHeader += "%-" + fmt.Sprintf("%d", nameSize) + "s "
	}

	header = append(header, standardHeaders...)

	connHeader += strings.Join(defaultHeaderColumns, "  ")
	if displaySubs {
		connHeader += "%13s"
	}

	connHeader += "\n" // ...LAST ACTIVITY

	var connRows string
	if displaySubs {
		header = append(header, "SUBSCRIPTIONS")
	}

	connRows = fmt.Sprintf(connHeader, header...)

	text += connRows // Add to screen!

	connValues := DEFAULT_PADDING

	connValues += "%-" + fmt.Sprintf("%d", hostSize) + "s " // HOST: e.g. 192.168.1.1:78901

	connValues += " %-6d " // CID: e.g. 1234

	if nameSize > 0 { // NAME: e.g. hello
		connValues += "%-" + fmt.Sprintf("%d", nameSize) + "s "
	}

	connValues += strings.Join(defaultRowColumns, "  ")
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

		var connLine string // Build the info line
		connLineInfo := make([]interface{}, 0)
		connLineInfo = append(connLineInfo, h)
		connLineInfo = append(connLineInfo, conn.Cid)

		if nameSize > 0 { // Name not included unless present
			connLineInfo = append(connLineInfo, conn.Name)
		}

		connLineInfo = append(connLineInfo, conn.NumSubs)

		connLineInfo = append(connLineInfo, top.Nsize(*displayRawBytes, int64(conn.Pending)))

		if !engine.ShowRates {
			connLineInfo = append(connLineInfo, top.Nsize(*displayRawBytes, conn.OutMsgs), top.Nsize(*displayRawBytes, conn.InMsgs))
			connLineInfo = append(connLineInfo, top.Psize(*displayRawBytes, conn.OutBytes), top.Psize(*displayRawBytes, conn.InBytes))
		} else {
			var (
				inMsgsPerSec   float64
				outMsgsPerSec  float64
				inBytesPerSec  float64
				outBytesPerSec float64
			)
			crate, wasConnected := stats.Rates.Connections[conn.Cid]
			if wasConnected {
				outMsgsPerSec = crate.OutMsgsRate
				inMsgsPerSec = crate.InMsgsRate
				outBytesPerSec = crate.OutBytesRate
				inBytesPerSec = crate.InBytesRate
			}
			connLineInfo = append(connLineInfo, top.Nsize(*displayRawBytes, int64(outMsgsPerSec)), top.Nsize(*displayRawBytes, int64(inMsgsPerSec)))
			connLineInfo = append(connLineInfo, top.Psize(*displayRawBytes, int64(outBytesPerSec)), top.Psize(*displayRawBytes, int64(inBytesPerSec)))
		}

		connLineInfo = append(connLineInfo, conn.Lang, conn.Version)
		connLineInfo = append(connLineInfo, conn.Uptime, conn.LastActivity)

		if displaySubs {
			subs := strings.Join(conn.Subs, ", ")
			connLineInfo = append(connLineInfo, subs)
		}

		connLine = fmt.Sprintf(connValues, connLineInfo...)

		text += connLine // Add line to screen!
	}

	return text
}

func generateParagraphCSV(
	engine *top.Engine,
	stats *top.Stats,
	delimiter string,
) string {

	defaultHeaderAndRowColumnsForCsv := []string{"%s", "%s", "%s", "%s", "%s", "%s", "%s", "%s", "%s", "%s"} // Chopped: HOST CID NAME...

	cpu := stats.Varz.CPU // Snapshot current stats
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

	mem := top.Psize(false, memVal) // memory is exempt from the rawbytes flag
	inMsgs := top.Nsize(*displayRawBytes, inMsgsVal)
	outMsgs := top.Nsize(*displayRawBytes, outMsgsVal)
	inBytes := top.Psize(*displayRawBytes, inBytesVal)
	outBytes := top.Psize(*displayRawBytes, outBytesVal)
	inMsgsRate := stats.Rates.InMsgsRate
	outMsgsRate := stats.Rates.OutMsgsRate
	inBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.InBytesRate))
	outBytesRate := top.Psize(*displayRawBytes, int64(stats.Rates.OutBytesRate))

	info := "NATS server version[__DELIM__]%s[__DELIM__](uptime: %s)[__DELIM__]%s\n"
	info += "Server:\n"
	info += "Load:[__DELIM__]CPU:[__DELIM__]%.1f%%[__DELIM__]Memory:[__DELIM__]%s[__DELIM__]Slow Consumers:[__DELIM__]%d\n"
	info += "In:[__DELIM__]Msgs:[__DELIM__]%s[__DELIM__]Bytes:[__DELIM__]%s[__DELIM__]Msgs/Sec:[__DELIM__]%.1f[__DELIM__]Bytes/Sec:[__DELIM__]%s\n"
	info += "Out:[__DELIM__]Msgs:[__DELIM__]%s[__DELIM__]Bytes:[__DELIM__]%s[__DELIM__]Msgs/Sec:[__DELIM__]%.1f[__DELIM__]Bytes/Sec:[__DELIM__]%s"

	text := fmt.Sprintf(
		info, serverVersion, uptime, stats.Error,
		cpu, mem, slowConsumers,
		inMsgs, inBytes, inMsgsRate, inBytesRate,
		outMsgs, outBytes, outMsgsRate, outBytesRate,
	)

	text += fmt.Sprintf("\n\nConnections Polled:[__DELIM__]%d\n", numConns)

	displaySubs := engine.DisplaySubs
	for _, conn := range stats.Connz.Conns {
		if !*lookupDNS {
			continue
		}

		_, present := resolvedHosts[conn.IP]
		if present {
			continue
		}

		addrs, err := net.LookupAddr(conn.IP)

		hostname := ""
		if err == nil && len(addrs) > 0 && len(addrs[0]) > 0 { // Make a lookup for each one of the ips and memoize them for subsequent polls
			hostname = addrs[0]
		} else { // Otherwise just continue to use ip:port as resolved host can be an empty string even though there were no errors
			hostname = fmt.Sprintf("%s:%d", conn.IP, conn.Port)
		}

		resolvedHosts[conn.IP] = hostname
	}

	header := make([]interface{}, 0) // Dynamically add columns
	connHeader := ""

	header = append(header, "HOST") // HOST
	connHeader += "%s[__DELIM__]"

	header = append(header, "CID") // CID
	connHeader += "%s[__DELIM__]"

	header = append(header, "NAME") // NAME
	connHeader += "%s[__DELIM__]"

	header = append(header, standardHeaders...)
	connHeader += strings.Join(defaultHeaderAndRowColumnsForCsv, "[__DELIM__]")

	if displaySubs {
		connHeader += "[__DELIM__]%s" // SUBSCRIPTIONS
	}

	connHeader += "\n" // ...LAST ACTIVITY

	if displaySubs {
		header = append(header, "SUBSCRIPTIONS")
	}

	text += fmt.Sprintf(connHeader, header...) // Add to screen!

	connValues := "%s[__DELIM__]" // HOST: e.g. 192.168.1.1:78901
	connValues += "%d[__DELIM__]" // CID: e.g. 1234
	connValues += "%s[__DELIM__]" // NAME: e.g. hello

	connValues += strings.Join(defaultHeaderAndRowColumnsForCsv, "[__DELIM__]")
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

		connLineInfo := make([]interface{}, 0)
		connLineInfo = append(connLineInfo, h)
		connLineInfo = append(connLineInfo, conn.Cid)
		connLineInfo = append(connLineInfo, conn.Name)
		connLineInfo = append(connLineInfo, fmt.Sprintf("%d", conn.NumSubs))
		connLineInfo = append(connLineInfo, top.Nsize(*displayRawBytes, int64(conn.Pending)), top.Nsize(*displayRawBytes, conn.OutMsgs), top.Nsize(*displayRawBytes, conn.InMsgs))
		connLineInfo = append(connLineInfo, top.Psize(*displayRawBytes, conn.OutBytes), top.Psize(*displayRawBytes, conn.InBytes))
		connLineInfo = append(connLineInfo, conn.Lang, conn.Version)
		connLineInfo = append(connLineInfo, conn.Uptime, conn.LastActivity)

		if displaySubs {
			subs := "[__DELIM__]" + strings.Join(conn.Subs, "  ") // its safer to use a couple of whitespaces instead of commas to separate the subs because comma is reserved to separate entire columns!
			connLineInfo = append(connLineInfo, subs)
		}

		text += fmt.Sprintf(connValues, connLineInfo...)
	}

	text = strings.ReplaceAll(text, "[__DELIM__]", delimiter)

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
	text := generateParagraph(engine, cleanStats, "")
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

			par.Text = generateParagraph(engine, stats, "") // Update top view text

			redraw <- DueToNewStats
		}
	}

	// Flags for capturing options
	waitingSortOption := false
	waitingLimitOption := false

	optionBuf := ""
	refreshOptionHeader := func() {
		clrline := fmt.Sprintf("%s                  ", UI_HEADER_PREFIX) // Need to mask what was typed before

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
							fmt.Printf("%sinvalid order: %s%s", UI_HEADER_PREFIX, optionBuf, emptyPadding)
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
				fmt.Printf("%ssort by [%s]: %s", UI_HEADER_PREFIX, engine.SortOpt, optionBuf)
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
				fmt.Printf("%slimit   [%d]: %s", UI_HEADER_PREFIX, engine.Conns, optionBuf)
			}

			if e.Type == ui.EventKey && e.Key == ui.KeySpace {
				engine.ShowRates = !engine.ShowRates
			}

			if e.Type == ui.EventKey && (e.Ch == 'q' || e.Key == ui.KeyCtrlC) {
				close(engine.ShutdownCh)
				cleanExit()
			}

			if e.Type == ui.EventKey && e.Ch == 's' && !(waitingLimitOption || waitingSortOption) {
				engine.DisplaySubs = !engine.DisplaySubs
			}

			if e.Type == ui.EventKey && viewMode == HelpViewMode {
				ui.Body.Rows = topViewGrid.Rows
				viewMode = TopViewMode
				continue
			}

			if e.Type == ui.EventKey && e.Ch == 'o' && !waitingLimitOption && viewMode == TopViewMode {
				fmt.Printf("%ssort by [%s]:", UI_HEADER_PREFIX, engine.SortOpt)
				waitingSortOption = true
			}

			if e.Type == ui.EventKey && e.Ch == 'n' && !waitingSortOption && viewMode == TopViewMode {
				fmt.Printf("%slimit   [%d]:", UI_HEADER_PREFIX, engine.Conns)
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
				*lookupDNS = !*lookupDNS
			}

			if e.Type == ui.EventKey && (e.Ch == 'b') && !(waitingSortOption || waitingLimitOption) {
				*displayRawBytes = !*displayRawBytes
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

space            Toggle displaying rates per second in connections.

q                Quit nats-top.

Press any key to continue...

`
	return text
}
