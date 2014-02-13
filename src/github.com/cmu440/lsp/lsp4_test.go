// LSP advanced buffering/synchronization tests.

// These tests are advanced in nature. Messages are "streamed" in large
// batches and the network is toggled on/off (i.e. drop percent is set to
// either 0% or 100%) throughout. The server/client Close methods are
// also tested in the presence of a unreliable network.

package lsp

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

type syncTestMode int

const (
	doServerFastClose syncTestMode = iota
	doServerToClient
	doClientToServer
	doRoundTrip
)

type syncTestSystem struct {
	t                   *testing.T
	server              Server
	clients             []Client
	clientMap           map[int]int // From connection id to client number.
	mapLock             sync.Mutex  // To protect the client map.
	params              *Params
	mode                syncTestMode
	port                int
	data                [][]int
	numClients          int
	numMsgs             int
	maxEpochs           int
	desc                string
	timeoutChan         <-chan time.Time
	clientToMasterChan  chan struct{} // For client to signal when its done.
	serverToMasterChan  chan struct{} // For server to signal when its done.
	networkToMasterChan chan struct{} // For network to signal when its done.
	masterToClientChan  chan struct{}
	masterToServerChan  chan struct{}
	masterToNetworkChan chan struct{}
	exitChan            chan struct{}
	errChan             chan error
}

func newSyncTestSystem(t *testing.T, numClients, numMsgs int, mode syncTestMode, params *Params) *syncTestSystem {
	ts := new(syncTestSystem)
	ts.t = t
	ts.mode = mode
	ts.params = params
	ts.numClients = numClients
	ts.numMsgs = numMsgs
	ts.clients = make([]Client, numClients)
	ts.clientMap = make(map[int]int, numClients)
	ts.data = make([][]int, numClients)
	for i := range ts.clients {
		result := make([]int, numMsgs)
		for j := range result {
			result[j] = rand.Int()
		}
		ts.data[i] = result
	}
	ts.numClients = numClients
	ts.port = 3000 + rand.Intn(50000)
	ts.clientToMasterChan = make(chan struct{})
	ts.serverToMasterChan = make(chan struct{})
	ts.networkToMasterChan = make(chan struct{})
	ts.masterToClientChan = make(chan struct{})
	ts.masterToServerChan = make(chan struct{})
	ts.masterToNetworkChan = make(chan struct{})
	ts.exitChan = make(chan struct{})
	ts.errChan = make(chan error, numClients+1)
	return ts
}

func (ts *syncTestSystem) setDescription(desc string) *syncTestSystem {
	ts.desc = desc
	return ts
}

func (ts *syncTestSystem) setMaxEpochs(maxEpochs int) *syncTestSystem {
	ts.maxEpochs = maxEpochs
	return ts
}

func (ts *syncTestSystem) runTest() {
	defer lspnet.ResetDropPercent()
	fmt.Printf("=== %s (%d clients, %d msgs/client, %d max epochs, %d window size)\n",
		ts.desc, ts.numClients, ts.numMsgs, ts.maxEpochs, ts.params.WindowSize)
	go ts.runNetwork()
	go ts.runServer()
	for i := range ts.clients {
		go ts.runClient(i)
	}
	ts.t.Logf("Setting test to timeout after %d epochs.", ts.maxEpochs)
	ts.timeoutChan = time.After(time.Duration(ts.maxEpochs*ts.params.EpochMillis) * time.Millisecond)
	ts.master()
	close(ts.exitChan)
}

// Alternates between turning network writes on and off in a loop.
// Runs in a background goroutine and is started once at the very beginning of a test.
func (ts *syncTestSystem) runNetwork() {
	t := ts.t
	// Network initially on
	t.Log("Setting global write drop percent to 0%")
	lspnet.SetWriteDropPercent(0)
	for {
		select {
		case <-ts.exitChan:
			t.Log("Received exit signal in runNetwork. Shutting down...")
			return
		default:
			t.Log("Waiting for master...")
			<-ts.masterToNetworkChan
			t.Log("Setting global write drop percent to 100%")
			lspnet.SetWriteDropPercent(100)
			ts.networkToMasterChan <- struct{}{}

			t.Log("Waiting for master...")
			<-ts.masterToNetworkChan
			t.Log("Setting global write drop percent to 0%")
			lspnet.SetWriteDropPercent(0)
			ts.networkToMasterChan <- struct{}{}
			t.Logf("Sleeping for %d ms", 2*ts.params.EpochMillis)
			time.Sleep(time.Duration(2*ts.params.EpochMillis) * time.Millisecond)
		}
	}
}

// Create and runs the server. Runs in a background goroutine and is started
// once at the very beginning of a test.
func (ts *syncTestSystem) runServer() {
	t := ts.t
	srv, err := NewServer(ts.port, ts.params)
	if err != nil {
		t.Errorf("Couldn't create server on port %d.", ts.port)
		ts.errChan <- fmt.Errorf("couldn't create server on port %d", ts.port)
		return
	}
	t.Logf("Server created on port %d.", ts.port)
	ts.server = srv
	ts.serverToMasterChan <- struct{}{}
	// Read
	if ts.mode != doServerToClient {
		t.Log("Server waiting to read...")
		<-ts.masterToServerChan
		t.Log("Server beginning to read messages...")
		// Receive messages from client. Track number received from each client.
		rcvdCount := make([]int, ts.numClients)
		for m := 0; m < ts.numMsgs*ts.numClients; m++ {
			id, b, err := ts.server.Read()
			if err != nil {
				t.Errorf("Server failed to read: %s\n", err)
				ts.errChan <- fmt.Errorf("server failed to read: %s", err)
				return
			}
			var v int
			json.Unmarshal(b, &v)
			ts.mapLock.Lock()
			clientID, ok := ts.clientMap[id]
			ts.mapLock.Unlock()
			if !ok || clientID >= ts.numClients {
				t.Errorf("server received message from unknown client %d", id)
				ts.errChan <- fmt.Errorf("server received message from unknown client %d", id)
				return
			}
			rc := rcvdCount[clientID]
			if rc >= ts.numMsgs {
				t.Errorf("server received too many messages from client %d", id)
				ts.errChan <- fmt.Errorf("server has received too many messages from client %d", id)
				return
			}
			ev := ts.data[clientID][rc]
			if v != ev {
				t.Errorf("Server received unexpected element")
				ts.errChan <- fmt.Errorf("server received unexpected element %v, value %v from client %v; expected %v",
					rc, v, id, ev)
				return
			}
			t.Logf("Server received element %v, value %v from connection %v. Correct!", rc, v, id)
			rcvdCount[clientID] = rc + 1
		}
		t.Logf("Server read all %d messages from clients.", ts.numMsgs*ts.numClients)
		ts.serverToMasterChan <- struct{}{}
	}
	// Write
	if ts.mode != doClientToServer {
		t.Log("Server waiting to write...")
		<-ts.masterToServerChan
		t.Log("Server writing messages...")
		// Track number sent to each client
		sentCount := make([]int, ts.numClients)
		for nsent := 0; nsent < ts.numMsgs*ts.numClients; {
			var clientID int
			// Choose random client
			for {
				clientID = rand.Intn(ts.numClients)
				if sentCount[clientID] < ts.numMsgs {
					break
				}
			}
			id := ts.clients[clientID].ConnID()
			sc := sentCount[clientID]
			v := ts.data[clientID][sc]
			b, _ := json.Marshal(v)
			err := ts.server.Write(id, b)
			if err != nil {
				t.Errorf("Server failed to write: %s", err)
				ts.errChan <- fmt.Errorf("server failed to send value %v (#%d) to client %d (ID %v)",
					v, sc, clientID, id)
				return

			}
			sentCount[clientID] = sc + 1
			nsent++
		}
		t.Logf("Server wrote all %d messages to clients", ts.numMsgs*ts.numClients)
		ts.serverToMasterChan <- struct{}{}
	}
	t.Log("Server waiting to close...")
	<-ts.masterToServerChan
	t.Log("Server closing...")
	ts.server.Close()
	t.Log("Server closed.")
	ts.serverToMasterChan <- struct{}{}
}

// Create and runs a client. Runs in a background goroutine. Called once per client at
// the very beginning of a test.
func (ts *syncTestSystem) runClient(clienti int) {
	t := ts.t
	hostport := lspnet.JoinHostPort("localhost", strconv.Itoa(ts.port))
	cli, err := NewClient(hostport, ts.params)
	if err != nil {
		t.Errorf("Couldn't create client %d to server at %s", clienti, hostport)
		ts.errChan <- fmt.Errorf("couldn't create client %d to server at %s", clienti, hostport)
		return
	}
	ts.clients[clienti] = cli
	id := cli.ConnID()
	ts.mapLock.Lock()
	ts.clientMap[id] = clienti
	ts.mapLock.Unlock()
	t.Logf("Client %d created with id %d to server at %s", clienti, id, hostport)
	ts.clientToMasterChan <- struct{}{}

	// Write
	if ts.mode != doServerToClient {
		t.Logf("Client %d (id %d) waiting to write\n", clienti, id)
		<-ts.masterToClientChan
		t.Logf("Client %d (id %d) writing messages\n", clienti, id)
		for nsent := 0; nsent < ts.numMsgs; nsent++ {
			v := ts.data[clienti][nsent]
			b, _ := json.Marshal(v)
			if err := cli.Write(b); err != nil {
				t.Errorf("Client failed to write: %s", err)
				ts.errChan <- fmt.Errorf("client %d (id %d) could not send value %v", clienti, id, v)
				return
			}
		}
		t.Logf("Client %d (id %d) wrote all %v messages to server\n", clienti, id, ts.numMsgs)
		ts.clientToMasterChan <- struct{}{}
	}
	// Read
	if ts.mode != doClientToServer {
		t.Logf("Client %d (id %d) waiting to read", clienti, id)
		<-ts.masterToClientChan
		t.Logf("Client %d (id %d) reading messages", clienti, id)
		// Receive messages from server
		for nrcvd := 0; nrcvd < ts.numMsgs; nrcvd++ {
			b, err := cli.Read()
			if err != nil {
				t.Errorf("Client failed to read: %s", err)
				ts.errChan <- fmt.Errorf("client %d (id %d) failed to read value #%d",
					clienti, id, nrcvd)
				return
			}
			var v int
			json.Unmarshal(b, &v)
			ev := ts.data[clienti][nrcvd]
			if v != ev {
				t.Errorf("Client %d (id %d) received element #%v, value %v. Expected %v\n",
					clienti, id, nrcvd, v, ev)
				ts.errChan <- fmt.Errorf("client %d (id %d) received element #%v, value %v; expected %v",
					clienti, id, nrcvd, v, ev)
				return
			}
			t.Logf("Client %d (id %d) received element #%v, value %v. Correct!",
				clienti, id, nrcvd, v)
		}
		t.Logf("Client %d (id %d) read all %v messages from servers\n", clienti, id, ts.numMsgs)
		ts.clientToMasterChan <- struct{}{}
	}
	t.Logf("Client %d (id %d) waiting to close...", clienti, id)
	<-ts.masterToClientChan
	t.Logf("Client %d (id %d) closing...", clienti, id)
	cli.Close()
	t.Logf("Client %d (id %d) done.", clienti, id)
	ts.clientToMasterChan <- struct{}{}
}

// Send signal to server. Called from the master's goroutine.
func (ts *syncTestSystem) signalServer() {
	ts.t.Log("Sending signal to server...")
	ts.masterToServerChan <- struct{}{}
}

// Send signal to all clients. Called from the master's goroutine.
func (ts *syncTestSystem) signalClients() {
	ts.t.Log("Sending signal to all clients...")
	for i := 0; i < ts.numClients; i++ {
		ts.masterToClientChan <- struct{}{}
	}
}

// Wait until we receive a notification from the server (or timeout).
// Called from the master's goroutine.
func (ts *syncTestSystem) waitForServer() {
	t := ts.t
	t.Log("Waiting for server...")
	select {
	case <-ts.timeoutChan:
		close(ts.exitChan)
		t.Fatalf("Test timed out waiting for server.")
	case err := <-ts.errChan:
		close(ts.exitChan)
		t.Fatalf("Test failed with error: %s", err)
	case <-ts.serverToMasterChan:
		t.Log("Got signal from server. Exiting...")
	}
}

// Wait until we receive a notification from all clients (or timeout).
// Called from the master's goroutine.
func (ts *syncTestSystem) waitForClients() {
	t := ts.t
	t.Log("Waiting for clients...")
	for i := 0; i < ts.numClients; i++ {
		select {
		case <-ts.timeoutChan:
			close(ts.exitChan)
			t.Fatalf("Test timed out waiting for clients.")
		case err := <-ts.errChan:
			close(ts.exitChan)
			t.Fatalf("Test failed with error: %s", err)
		case <-ts.clientToMasterChan:
			t.Log("Got signals from all clients. Exiting...")
		}
	}
}

// Toggle the network and wait until sends back a notification that it is done.
// Called from the master's goroutine.
func (ts *syncTestSystem) toggleNetwork() {
	t := ts.t
	t.Log("Toggling network")
	ts.masterToNetworkChan <- struct{}{}
	t.Log("Waiting for network...")
	select {
	case <-ts.timeoutChan:
		close(ts.exitChan)
		t.Fatalf("Test timed out.")
	case <-ts.networkToMasterChan:
		t.Log("Network toggled.")
	}
}

// Master method to coordinate the test.
func (ts *syncTestSystem) master() {
	t := ts.t
	// Wait until server and all clients are ready
	ts.waitForServer()
	ts.waitForClients()
	// Network is initially off. Send signal to shutdown network...
	t.Log("Server + all clients started. Shutting off network...")
	ts.toggleNetwork()
	// Network Off
	// Enable client writes
	if ts.mode != doServerToClient {
		ts.signalClients()
		ts.waitForClients()
	}
	// Do fast close of client. The calls will not be able to complete.
	if ts.mode == doClientToServer {
		ts.signalClients()
	}
	if ts.mode != doServerToClient {
		// Turn on network and delay
		ts.toggleNetwork()
		if ts.mode == doClientToServer {
			ts.waitForClients()
		}
		// Turn network off
		ts.toggleNetwork()
		// Network off
		// Enable server reads
		ts.signalServer()
		ts.waitForServer()
	}
	// Do server writes
	if ts.mode != doClientToServer {
		ts.signalServer()
		ts.waitForServer()
	}
	// Do fast close of server. The calls will not be able to complete.
	if ts.mode != doRoundTrip {
		ts.signalServer()
	}
	if ts.mode != doClientToServer {
		// Turn on network.
		ts.toggleNetwork()
		if ts.mode != doRoundTrip {
			// If did quick close, should get responses from server.
			ts.waitForServer()
		}
		// Network off again
		ts.toggleNetwork()
		// Enable client reads
		ts.signalClients()
		ts.waitForClients()
		// Enable client closes
		ts.signalClients()
		ts.waitForClients()
	}
	// Final close by server
	if ts.mode == doRoundTrip {
		ts.signalServer()
		ts.waitForServer()
	}
	// Made it!
}

func TestServerFastClose1(t *testing.T) {
	newSyncTestSystem(t, 1, 10, doServerFastClose, &Params{5, 500, 1}).
		setDescription("TestServerFastClose1: Fast close of server").
		setMaxEpochs(12).
		runTest()
}

func TestServerFastClose2(t *testing.T) {
	newSyncTestSystem(t, 3, 10, doServerFastClose, &Params{5, 500, 1}).
		setDescription("TestServerFastClose2: Fast close of server").
		setMaxEpochs(12).
		runTest()
}

func TestServerFastClose3(t *testing.T) {
	newSyncTestSystem(t, 5, 500, doServerFastClose, &Params{5, 2000, 1}).
		setDescription("TestServerFastClose3: Fast close of server").
		setMaxEpochs(20).
		runTest()
}

func TestServerToClient1(t *testing.T) {
	newSyncTestSystem(t, 1, 10, doServerToClient, &Params{5, 500, 1}).
		setDescription("TestServerToClient1: Stream from server to client").
		setMaxEpochs(12).
		runTest()
}

func TestServerToClient2(t *testing.T) {
	newSyncTestSystem(t, 3, 10, doServerToClient, &Params{5, 500, 1}).
		setDescription("TestServerToClient2: Stream from server to client").
		setMaxEpochs(12).
		runTest()
}

func TestServerToClient3(t *testing.T) {
	newSyncTestSystem(t, 5, 500, doServerToClient, &Params{5, 2000, 1}).
		setDescription("TestServerToClient3: Stream from server to client").
		setMaxEpochs(20).
		runTest()
}

func TestClientToServer1(t *testing.T) {
	newSyncTestSystem(t, 1, 10, doClientToServer, &Params{5, 500, 1}).
		setDescription("TestClientToServer1: Stream from client to server").
		setMaxEpochs(12).
		runTest()
}

func TestClientToServer2(t *testing.T) {
	newSyncTestSystem(t, 3, 10, doClientToServer, &Params{5, 500, 1}).
		setDescription("TestClientToServer2: Stream from client to server").
		setMaxEpochs(12).
		runTest()
}

func TestClientToServer3(t *testing.T) {
	newSyncTestSystem(t, 5, 500, doClientToServer, &Params{5, 2000, 1}).
		setDescription("TestClientToServer3: Stream from client to server").
		setMaxEpochs(20).
		runTest()
}

func TestRoundTrip1(t *testing.T) {
	newSyncTestSystem(t, 1, 10, doRoundTrip, &Params{5, 500, 1}).
		setDescription("TestRoundTrip1: Buffered msgs in client and server").
		setMaxEpochs(12).
		runTest()
}

func TestRoundTrip2(t *testing.T) {
	newSyncTestSystem(t, 3, 10, doRoundTrip, &Params{5, 500, 1}).
		setDescription("TestRoundTrip2: Buffered msgs in client and server").
		setMaxEpochs(12).
		runTest()
}

func TestRoundTrip3(t *testing.T) {
	newSyncTestSystem(t, 5, 500, doRoundTrip, &Params{5, 2000, 1}).
		setDescription("TestRoundTrip3: Buffered msgs in client and server").
		setMaxEpochs(20).
		runTest()
}
