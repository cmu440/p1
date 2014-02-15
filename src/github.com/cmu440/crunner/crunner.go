// Implementation of an echo client based on LSP.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"

	"github.com/cmu440/lsp"
	"github.com/cmu440/lspnet"
)

var (
	port        = flag.Int("port", 9999, "server port number")
	host        = flag.String("host", "localhost", "server host address")
	readDrop    = flag.Int("rdrop", 0, "network read drop percent")
	writeDrop   = flag.Int("wdrop", 0, "network write drop percent")
	epochLimit  = flag.Int("elim", lsp.DefaultEpochLimit, "epoch limit")
	epochMillis = flag.Int("ems", lsp.DefaultEpochMillis, "epoch duration (ms)")
	windowSize  = flag.Int("wsize", lsp.DefaultWindowSize, "window size")
	showLogs    = flag.Bool("v", false, "show crunner logs")
)

func init() {
	// Display time, file, and line number in log messages.
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
}

func main() {
	flag.Parse()
	if !*showLogs {
		log.SetOutput(ioutil.Discard)
	} else {
		lspnet.EnableDebugLogs(true)
	}
	lspnet.SetClientReadDropPercent(*readDrop)
	lspnet.SetClientWriteDropPercent(*writeDrop)
	params := &lsp.Params{
		EpochLimit:  *epochLimit,
		EpochMillis: *epochMillis,
		WindowSize:  *windowSize,
	}
	hostport := lspnet.JoinHostPort(*host, strconv.Itoa(*port))
	fmt.Printf("Connecting to server at '%s'...\n", hostport)
	cli, err := lsp.NewClient(hostport, params)
	if err != nil {
		fmt.Printf("Failed to connect to server at %s: %s\n", hostport, err)
		return
	}
	runClient(cli)
}

func runClient(cli lsp.Client) {
	defer fmt.Println("Exiting...")
	for {
		// Get next token from input.
		fmt.Printf("Client: ")
		var s string
		if _, err := fmt.Scan(&s); err != nil {
			return
		}
		// Send message to server.
		if err := cli.Write([]byte(s)); err != nil {
			fmt.Printf("Client %d failed to write to server: %s\n", cli.ConnID(), err)
			return
		}
		log.Printf("Client %d wrote '%s' to server\n", cli.ConnID(), s)
		// Read message from server.
		payload, err := cli.Read()
		if err != nil {
			fmt.Printf("Client %d failed to read from server: %s\n", cli.ConnID(), err)
			return
		}
		fmt.Printf("Server: %s\n", string(payload))
	}
}
