// LSP basic tests.

// These tests start a server and possibly multiple clients and runs
// a simple echo server. All messages sent by one side must be received
// by the other. Messages read by the other are then immediately written
// back. Some tests also test robustness by inserting random delays in
// between client/server reads or writes, and by increasing the packet
// loss to up to 20%. Lastly, TestSendReceive* test that all messages
// sent from one side are received by the other (without relying on
// epochs to resend any messages).

package lsp

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/cmu440/lspnet"
)

type testSystem struct {
	t              *testing.T
	server         Server
	clients        []Client
	exitChan       chan struct{}
	numClients     int
	numMsgs        int
	maxSleepMillis int
	desc           string
	dropPercent    int
	params         *Params
}

func (ts *testSystem) setMaxSleepMillis(ms int) *testSystem {
	ts.maxSleepMillis = ms
	return ts
}

func (ts *testSystem) setNumMsgs(n int) *testSystem {
	ts.numMsgs = n
	return ts
}

func (ts *testSystem) setDescription(desc string) *testSystem {
	ts.desc = desc
	return ts
}

func (ts *testSystem) setDropPercent(p int) *testSystem {
	ts.dropPercent = p
	return ts
}

func newTestSystem(t *testing.T, numClients int, params *Params) *testSystem {
	ts := new(testSystem)
	ts.t = t
	ts.params = params
	ts.numClients = numClients
	ts.exitChan = make(chan struct{})

	// Try to create server for different port numbers.
	const numTries = 5
	var port int
	var err error
	for i := 0; i < numTries && ts.server == nil; i++ {
		port = 3000 + rand.Intn(50000)
		ts.server, err = NewServer(port, params)
		if err != nil {
			t.Logf("Failed to start server on port %d: %s", port, err)
		}
	}
	if err != nil {
		t.Fatalf("Failed to start server.")
	}
	t.Logf("Started server on port %d.", port)

	// Create the clients.
	ts.clients = make([]Client, numClients)
	for i := range ts.clients {
		hostport := lspnet.JoinHostPort("localhost", strconv.Itoa(port))
		ts.clients[i], err = NewClient(hostport, params)
		if err != nil {
			t.Fatalf("Client failed to connect to server on port %d: %s.", port, err)
		}
	}
	t.Logf("Started %d clients.", numClients)
	return ts
}

// runServer sets up the server and reads/echos messages back to clients.
func (ts *testSystem) runServer() {
	defer ts.t.Log("Server shutting down...")
	for {
		select {
		case <-ts.exitChan:
			return
		default:
			connID, data, err := ts.server.Read()
			if err != nil {
				ts.t.Logf("Server received error during read.")
				return
			}
			ts.t.Logf("Server read message %s from client %d.", string(data), connID)
			ts.randSleep()
			ts.t.Logf("Server writing %s.", string(data))
			ts.server.Write(connID, data)
		}
	}
}

func (ts *testSystem) randSleep() {
	if ts.maxSleepMillis > 0 {
		time.Sleep(time.Duration(rand.Intn(ts.maxSleepMillis)) * time.Millisecond)
	}
}

// Runs a simple echo server and tests whether messages are correctly
// sent and received. Some tests introduce up to 20% packet loss,
// random delays, multiple clients, and large window sizes.
func (ts *testSystem) runClient(clientID int, doneChan chan<- bool) {
	defer ts.t.Log("Client shutting down...")
	cli := ts.clients[clientID]
	connID := cli.ConnID()
	for i := 0; i < ts.numMsgs; i++ {
		select {
		case <-ts.exitChan:
			return
		default:
			wt := rand.Intn(100)
			writeBytes, _ := json.Marshal(i + wt)
			err := cli.Write(writeBytes)
			ts.t.Logf("Client %d wrote message %d (%d of %d).", connID, i+wt, i+1, ts.numMsgs)
			if err != nil {
				ts.t.Errorf("Client %d write got error: %s.", connID, err)
				doneChan <- false
				return
			}
			readBytes, err := cli.Read()
			if err != nil {
				ts.t.Errorf("Client %d read got error: %s.", connID, err)
				doneChan <- false
				return
			}
			ts.t.Logf("Client %d read message %s (%d of %d).", connID, string(readBytes), i+1, ts.numMsgs)
			var v int
			json.Unmarshal(readBytes, &v)
			if v != wt+i {
				ts.t.Errorf("Client %d received message %d, expected %d.", connID, v, i+wt)
				doneChan <- false
				return
			}
		}
	}
	ts.t.Logf("Client %d completed %d messages\n", connID, ts.numMsgs)
	doneChan <- true
}

func (ts *testSystem) runTest(timeout int) {
	lspnet.SetWriteDropPercent(ts.dropPercent)
	defer lspnet.ResetDropPercent()

	fmt.Printf("=== %s (%d clients, %d msgs/client, %d%% drop rate, %d window size)\n",
		ts.desc, ts.numClients, ts.numMsgs, ts.dropPercent, ts.params.WindowSize)

	clientDoneChan := make(chan bool, ts.numClients)
	go ts.runServer()
	for i := range ts.clients {
		go ts.runClient(i, clientDoneChan)
	}
	timeoutChan := time.After(time.Duration(timeout) * time.Millisecond)
	for _ = range ts.clients {
		select {
		case <-timeoutChan:
			close(ts.exitChan)
			ts.t.Fatalf("Test timed out after %.2f secs", float64(timeout)/1000.0)
		case ok := <-clientDoneChan:
			if !ok {
				// May or may not close the server goroutine. There's no guarantee
				// since we do not explicitly test the students' Close implementations
				// in these basic tests.
				close(ts.exitChan)
				ts.t.Fatal("Client failed due to an error.")
			}
		}
	}
	close(ts.exitChan)
}

func makeParams(epochLimit, epochMillis, windowSize int) *Params {
	return &Params{
		EpochLimit:  epochLimit,
		EpochMillis: epochMillis,
		WindowSize:  windowSize,
	}
}

func TestBasic1(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1)).
		setDescription("TestBasic1: Short client/server interaction").
		setNumMsgs(3).
		runTest(2000)
}

func TestBasic2(t *testing.T) {
	newTestSystem(t, 1, makeParams(5, 2000, 1)).
		setDescription("TestBasic2: Long client/server interaction").
		setNumMsgs(50).
		runTest(2000)
}

func TestBasic3(t *testing.T) {
	newTestSystem(t, 2, makeParams(5, 2000, 1)).
		setDescription("TestBasic3: Two client/one server interaction").
		setNumMsgs(50).
		runTest(2000)
}

func TestBasic4(t *testing.T) {
	newTestSystem(t, 10, makeParams(5, 2000, 2)).
		setDescription("TestBasic4: Ten clients, long interaction").
		setNumMsgs(50).
		runTest(2000)
}

func TestBasic5(t *testing.T) {
	newTestSystem(t, 2, makeParams(5, 2000, 2)).
		setDescription("TestBasic5: Two clients, 500 messages").
		setNumMsgs(500).
		runTest(2000)
}

func TestBasic6(t *testing.T) {
	newTestSystem(t, 10, makeParams(5, 2000, 20)).
		setDescription("TestBasic6: Ten clients, 500 messages").
		setNumMsgs(500).
		runTest(15000)
}

func TestBasic7(t *testing.T) {
	newTestSystem(t, 4, makeParams(5, 2000, 2)).
		setDescription("TestBasic7: Random delays by clients & server").
		setNumMsgs(10).
		setMaxSleepMillis(100).
		runTest(15000)
}

func TestBasic8(t *testing.T) {
	newTestSystem(t, 5, makeParams(5, 2000, 10)).
		setDescription("TestBasic8: Random delays by clients & server").
		setNumMsgs(10).
		setMaxSleepMillis(100).
		runTest(15000)
}

func TestBasic9(t *testing.T) {
	newTestSystem(t, 2, makeParams(5, 2000, 10)).
		setDescription("TestBasic9: Random delays by clients & server").
		setNumMsgs(50).
		setMaxSleepMillis(100).
		runTest(15000)
}

func TestSendReceive1(t *testing.T) {
	newTestSystem(t, 1, makeParams(3, 5000, 1)).
		setDescription("TestSendReceive1: No epochs, single client").
		setNumMsgs(6).
		runTest(5000)
}

func TestSendReceive2(t *testing.T) {
	newTestSystem(t, 4, makeParams(3, 5000, 1)).
		setDescription("TestSendReceive2: No epochs, multiple clients").
		setNumMsgs(6).
		runTest(5000)
}

func TestSendReceive3(t *testing.T) {
	newTestSystem(t, 4, makeParams(3, 10000, 1)).
		setDescription("TestSendReceive3: No epochs, random delays inserted").
		setNumMsgs(6).
		setMaxSleepMillis(100).
		runTest(10000)
}

func TestRobust1(t *testing.T) {
	newTestSystem(t, 1, makeParams(20, 50, 1)).
		setDescription("TestRobust1: Single client, some packet dropping").
		setDropPercent(20).
		setNumMsgs(10).
		runTest(15000)
}

func TestRobust2(t *testing.T) {
	newTestSystem(t, 3, makeParams(20, 50, 1)).
		setDescription("TestRobust2: Three clients, some packet dropping").
		setDropPercent(20).
		setNumMsgs(15).
		runTest(15000)
}

func TestRobust3(t *testing.T) {
	newTestSystem(t, 5, makeParams(20, 50, 1)).
		setDescription("TestRobust3: Five clients, some packet dropping").
		setDropPercent(20).
		setNumMsgs(10).
		runTest(15000)
}

func TestRobust4(t *testing.T) {
	newTestSystem(t, 1, makeParams(20, 50, 2)).
		setDescription("TestRobust4: Single client, some packet dropping").
		setDropPercent(20).
		setNumMsgs(10).
		runTest(15000)
}

func TestRobust5(t *testing.T) {
	newTestSystem(t, 3, makeParams(20, 50, 5)).
		setDescription("TestRobust5: Three clients, some packet dropping").
		setDropPercent(20).
		setNumMsgs(15).
		runTest(15000)
}

func TestRobust6(t *testing.T) {
	newTestSystem(t, 5, makeParams(20, 50, 10)).
		setDescription("TestRobust6: Five clients, some packet dropping").
		setDropPercent(20).
		setNumMsgs(10).
		runTest(15000)
}
