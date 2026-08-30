package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/jeffwilliams/anvil/api/go/anvil"

	"github.com/ogier/pflag"
)

var (
	optInterval = pflag.IntP("interval", "i", 30, "Interval in seconds between dumps")
	optVerbose  = pflag.BoolP("verbose", "v", false, "Print extra information")
	optDumpfile = pflag.StringP("dumpfile", "f", "anvil-auto.dump", "Name of the dumpfile")
	optMaxLatency = pflag.IntP("max-latency", "l", 2000, "Max latency for API calls allowed in milliseconds, above which dumps are not taken")
)

func main() {
	pflag.Parse()

	anvil, err := anvil.NewFromEnv()
	if err != nil {
		fmt.Printf("autodump: %v", err)
		os.Exit(1)
	}

	b := []byte(fmt.Sprintf(`{"cmd": "Dump", "args": ["%s"], "winid": -1}`, *optDumpfile))
	cmd := bytes.NewReader(b)

	interval := time.Duration(*optInterval) * time.Second
	if *optVerbose {
		fmt.Printf("autodump: started. Interval is %d seconds\n", *optInterval)
	}

	for {
		time.Sleep(interval)

		l := latency(anvil)
		if l > time.Duration(*optMaxLatency)*time.Millisecond {
			continue
		}

		if *optVerbose {
			fmt.Printf("autodump: dumping\n")
		}
		_, err := anvil.Post("/execute", cmd)
		if err != nil {
			fmt.Printf("autodump: saving dumpfile failed: %v\n", err)
		}
		cmd.Reset(b)
	}

}

// The latency check here is to try and solve an issue. After leaving Anvil running with autodump
// and the computer idle for several hours, when Anvil was focused again it would be unresponsive.
// The logs show that it was trying to run the Dump command repeatedly with no delays between them.
// I believe that it could be that Anvil was swapped out and then once it was activated while it was 
// swapping in several queued requests from autodump would try to be executed all at once leading 
// to Anvil running Dump repeatedly and not responding to UI events. 
//
// I think by checking latency first with a simple in-memory request to Anvil before calling dump
// we should prevent Anvil from getting flooded by Dump requests if it is not ready for them. Also
// this can act as an additional throttle where if Anvil executes a requested Dump and it is very slow 
// (say because of the file not being in an OS file buffer) then the latency check should prevent 
// further concurrent Dump requests.
func latency(anvil anvil.Anvil) time.Duration {
	t1 := time.Now()
	anvil.Info()
	t2 := time.Now()
	return t2.Sub(t1)
}
