// Implementation of an echo server based on LSP.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/cmu440/lsp"
	"github.com/cmu440/lspnet"
)

var (
	port        = flag.Int("port", 9999, "port number")
	readDrop    = flag.Int("rdrop", 0, "network read drop percent")
	writeDrop   = flag.Int("wdrop", 0, "network write drop percent")
	epochLimit  = flag.Int("elim", lsp.DefaultEpochLimit, "epoch limit")
	epochMillis = flag.Int("ems", lsp.DefaultEpochMillis, "epoch duration (ms)")
	windowSize  = flag.Int("wsize", lsp.DefaultWindowSize, "window size")
	showLogs    = flag.Bool("v", false, "show srunner logs")
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
	lspnet.SetServerReadDropPercent(*readDrop)
	lspnet.SetServerWriteDropPercent(*writeDrop)
	params := &lsp.Params{
		EpochLimit:  *epochLimit,
		EpochMillis: *epochMillis,
		WindowSize:  *windowSize,
	}
	fmt.Printf("Starting server on port %d...\n", *port)
	srv, err := lsp.NewServer(*port, params)
	if err != nil {
		fmt.Printf("Failed to start Server on port %d: %s\n", *port, err)
		return
	}
	fmt.Println("Server waiting for clients...")
	runServer(srv)
}

func runServer(srv lsp.Server) {
	for {
		// Read message from client.
		if id, payload, err := srv.Read(); err != nil {
			fmt.Printf("Client %d has died: %s\n", id, err)
		} else {
			log.Printf("Server received '%s' from client %d\n", string(payload), id)
			// Echo message back to client.
			if err := srv.Write(id, payload); err != nil {
				// Print an error message and continue...
				fmt.Printf("Server failed to write to connection %d: %s\n", id, err)
			} else {
				log.Printf("Server wrote '%s' to client %d\n", string(payload), id)
			}
		}
	}
}
