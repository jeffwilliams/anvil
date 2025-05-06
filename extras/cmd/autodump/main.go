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
