package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/ogier/pflag"
)

var (
	noBody   io.Reader
	httpApi  anvil.Anvil
	cmds     = []string{}
	watchWin anvil.Window
)

var (
	optDebug = pflag.BoolP("debug", "d", false, "Print debug messages")
)

func main() {
	pflag.Parse()

	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handlePutNotification,
	}

	wsApi, err := httpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")

	loadFirstCommand()
	watchWin = findOrCreateWindow(&httpApi, watchPath())

	runCmdsAndUpdateWindow()

	wsApi.Run()
}

func debug(format string, args ...interface{}) {
	if !*optDebug {
		return
	}
	fmt.Printf(format, args...)
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "awatch: %s\n", msg)
	os.Exit(1)
}

func loadFirstCommand() {
	if len(pflag.Args()) < 1 {
		die("no arguments were passed. The arguments must be a command to run")
	}

	cmds = append(cmds, strings.Join(pflag.Args(), " "))
}

func run(cmd string) (output []byte, err error) {
	c := newCmd(cmd)
	return c.CombinedOutput()
}

func findOrCreateWindow(api *anvil.Anvil, watchPath string) anvil.Window {
	var wins []anvil.Window
	err := api.GetInto("/wins", &wins)
	dieIfError(err, "reading windows failed")
	for _, w := range wins {
		if w.Path == watchPath {
			return w
		}
	}

	win := createNewWindow(api)
	setWindowTag(api, win.Id, watchPath)
	return win
}

func createNewWindow(api *anvil.Anvil) anvil.Window {
	rsp, err := api.Post("/wins", noBody)
	dieIfError(err, "creating new window failed")

	raw, err := ioutil.ReadAll(rsp.Body)
	dieIfError(err, "reading response from creating window failed")

	var win anvil.Window
	err = json.Unmarshal(raw, &win)
	dieIfError(err, "decoding JSON response after creating window failed")
	return win
}

func watchPath() string {
	anvilGlobalPath := os.Getenv("ANVIL_WIN_GLOBAL_DIR")
	return filepath.Join(anvilGlobalPath, "+watch")
}

func setWindowTag(anvil *anvil.Anvil, winId int, watchPath string) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s Del! Snarf | Look ", watchPath)
	anvil.Put(fmt.Sprintf("/wins/%d/tag", winId), &buf)
}

func handlePutNotification(notif *anvil.Notification, err error) {
	if err != nil {
		// Parsing notification failed.
		fmt.Fprintf(os.Stderr, "awatch: parsing notification failed: %v\n", err)
		return
	}

	if notif.Op != anvil.NotificationOpPut {
		return
	}

	debug("awatch: got put notification for window %d\n", notif.WinId)

	var info anvil.Window
	err = httpApi.GetInto(fmt.Sprintf("/wins/%d/info", notif.WinId), &info)
	if err != nil {
		// Parsing notification failed.
		fmt.Fprintf(os.Stderr, "awatch: getting info for window %d failed: %v\n", notif.WinId, err)
		return
	}

	e := os.Getenv("ANVIL_WIN_LOCAL_DIR")
	localDir, err := filepath.Abs(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: getting absolute path of %s failed: %v\n", e, os.Getenv("ANVIL_WIN_LOCAL_DIR"))
		return
	}

	winPath, err := filepath.Abs(info.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: getting absolute path of %s failed: %v\n", info.Path, err)
		return
	}

	if !strings.HasPrefix(winPath, localDir) {
		debug("awatch: %s doesn't match our dir %s\n", winPath, localDir)
		return
	}

	updateCmdsFromWindow()
	runCmdsAndUpdateWindow()
}

// updateCmdsFromWindow reads all the lines from the window body that begin with '% ' and
// treat them as commands to run.
func updateCmdsFromWindow() {
	body, err := httpApi.WindowBody(watchWin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: reading watch window failed: %v\n", err)
		return
	}

	cmds = cmds[:0]
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "% ") {
			cmds = append(cmds, line[2:])
		}
	}
}

func runCmdsAndUpdateWindow() {
	debug("awatch: running commands\n")
	outputChans := make([]chan []byte, len(cmds))

	for i, cmd := range cmds {
		var err error
		debug("awatch: starting %s\n", cmd)
		outputChans[i], err = startCommandAndSendOutput(cmd)
		debug("awatch: started %s\n", cmd)
		if err != nil {
			writeErrorToChanAndClose(err, outputChans[i])
		}
	}

	outputBufs := make([]bytes.Buffer, len(cmds))

	var appender appender
	allClosed := false
	for !allClosed {
		time.Sleep(100 * time.Millisecond)
		lensBefore := calcLenOfBufs(outputBufs)
		allClosed = appender.appendAvailableOutputsInto(outputChans, outputBufs)
		lensAfter := calcLenOfBufs(outputBufs)
		if lensBefore.equal(lensAfter) {
			// no change in output. don't send to Anvil
			continue
		}
		contents := buildWindowContents(outputBufs)
		httpApi.Put(fmt.Sprintf("/wins/%d/body", watchWin.Id), contents)
	}

	debug("awatch: outputs all closed\n")

	return
}

func writeErrorToChanAndClose(err error, ch chan []byte) {
	ch <- []byte(err.Error())
	close(ch)
}

func startCommandAndSendOutput(cmd string) (ch chan []byte, err error) {
	ch = make(chan []byte)
	var stdout io.Reader
	var stderr io.Reader

	c := newCmd(cmd)

	stdout, err = c.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err = c.StderrPipe()
	if err != nil {
		return
	}

	err = c.Start()
	if err != nil {
		return
	}

	c1, c2 := mergeContentsInto(ch)
	go copyBlocks(stdout, c1, 1024)
	go copyBlocks(stderr, c2, 1024)
	return
}

func copyBlocks(source io.Reader, dest chan []byte, blocksize int) {
	defer close(dest)

	count := 0
	updateBlockSize := func() {
		if blocksize >= 1048576 {
			return
		}

		if count < 50 {
			count++
			return
		}

		blocksize = 1048576
	}

	for {
		block := make([]byte, blocksize)
		n, err := source.Read(block)

		if err != nil {
			if err != io.EOF {
				dest <- []byte(err.Error())
			}
			break
		}

		if n == 0 {
			continue
		}

		b := block
		if n < len(block) {
			b = block[:n]
		}
		dest <- b

		updateBlockSize()
	}

}

func mergeContentsInto(dest chan []byte) (c1, c2 chan []byte) {
	c1 = make(chan []byte)
	c2 = make(chan []byte)

	go func() {
		var eofs [2]bool
		for !(eofs[0] && eofs[1]) {
			select {
			case b, ok := <-c1:
				if !ok {
					eofs[0] = true
					c1 = nil
					continue
				}
				dest <- b
			case b, ok := <-c2:
				if !ok {
					eofs[1] = true
					c2 = nil
					continue
				}
				dest <- b
			}
		}
		close(dest)
	}()
	return
}

type appender struct {
	closed []bool
}

func (a *appender) appendAvailableOutputsInto(outputs []chan []byte, bufs []bytes.Buffer) (allClosed bool) {
	if a.closed == nil {
		a.closed = make([]bool, len(outputs))
	}

	for i, o := range outputs {
		select {
		case b, ok := <-o:
			if !ok {
				a.closed[i] = true
				break
			}
			bufs[i].Write(b)
		default:
		}
	}

	for _, b := range a.closed {
		if !b {
			return false
		}
	}

	return true
}

func buildWindowContents(outputs []bytes.Buffer) (contents *bytes.Buffer) {
	contents = new(bytes.Buffer)

	for i, c := range cmds {
		fmt.Fprintf(contents, "%% %s\n", c)
		contents.Write(outputs[i].Bytes())
	}

	return
}

type lenOfBufs []int

func (l lenOfBufs) equal(o lenOfBufs) bool {
	for i, v := range l {
		if v != o[i] {
			return false
		}
	}
	return true
}

func calcLenOfBufs(bufs []bytes.Buffer) lenOfBufs {
	l := make(lenOfBufs, len(bufs))
	for i, b := range bufs {
		l[i] = b.Len()
	}
	return l
}
