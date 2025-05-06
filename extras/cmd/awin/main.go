package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/ogier/pflag"
)

var (
	noBody       io.Reader
	anvilHttpApi anvil.Anvil
	anvilWsApi   anvil.Websock
	ttyWinId     int
	isTerminated func() bool
	doDebug      = true
)

var (
	optDebug      = pflag.BoolP("debug", "d", false, "Print debug messages")
	optEnablePty  = pflag.BoolP("enable-pty", "t", false, "Enable pseudo-terminal allocation. This is enabled by default on Linux and Mac.")
	optDisablePty = pflag.BoolP("disable-pty", "T", false, "Disable pseudo-terminal allocation. This is disabled by default on Windows.")
)

var logfile *os.File = nil

func debug(format string, args ...interface{}) {
	if !doDebug {
		return
	}
	fmt.Printf(format, args...)
}

func main() {
	parseOpts()

	cmdArgv, err := commandAndArgsToRun()
	if err != nil {
		usage()
		os.Exit(1)
	}

	cmdStdin, cmdStdout, f, err := startCmd(cmdArgv)
	isTerminated = f
	dieIfError(err, fmt.Sprintf("awin: Starting command failed: %v\n", err))

	anvilGlobalPath := os.Getenv("ANVIL_WIN_GLOBAL_PATH")
	if anvilGlobalPath == "" {
		var err error
		anvilGlobalPath, err = os.Getwd()
		dieIfError(err, fmt.Sprintf("awin: Environment variable ANVIL_WIN_GLOBAL_PATH is not set and getting current dir failed"))
	}

	anvilHttpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	notifChan, lastLineChan, clearLastLineChan, procOutputChan := setupPlumbing()

	handlers := anvil.WebsockHandlers{
		Notification: readNotifs(notifChan),
	}

	anvilWsApi, err = anvilHttpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")

	registerSendCommand(&anvilHttpApi)

	compoundPath := compoundPathForTag(anvilGlobalPath, cmdArgv)
	win := findOrCreateWindow(&anvilHttpApi, compoundPath)
	ttyWinId = win.Id

	go anvilWsApi.Run()
	go readProcess(cmdStdout, procOutputChan)
	np := NewNotificationProcessor(cmdStdin, notifChan, lastLineChan, clearLastLineChan)
	go np.run()
	oh := NewProcessOutputHandler(ttyWinId, procOutputChan, lastLineChan)
	go clearLastLine(&oh, clearLastLineChan)
	oh.run()
}

func setupPlumbing() (
	notifChan chan anvil.Notification,
	lastLineChan chan string,
	clearLastLineChan chan struct{},
	procOutputChan chan []byte,

) {
	notifChan = make(chan anvil.Notification)
	lastLineChan = make(chan string)
	clearLastLineChan = make(chan struct{})
	procOutputChan = make(chan []byte)
	return
}

var ptyAllocated bool

func startCmd(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	usePty := true
	if runtime.GOOS == "windows" {
		usePty = false
	}

	if *optDisablePty {
		usePty = false
	} else if *optEnablePty {
		usePty = true
	}

	ptyAllocated = usePty
	if usePty {
		return startCmdUsingPty(argv)
	} else {
		return startCmdWithoutPty(argv)
	}
}

func startCmdWithoutPty(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	var stdoutWriter, stdinReader *os.File
	stdout, stdoutWriter, err = os.Pipe()
	if err != nil {
		return
	}
	stdinReader, stdin, err = os.Pipe()
	if err != nil {
		return
	}

	procAttr := os.ProcAttr{
		Files: []*os.File{stdinReader, stdoutWriter, stdoutWriter},
	}

	var path string
	path, err = exec.LookPath(argv[0])
	if err != nil {
		return
	}

	process, err := os.StartProcess(path, argv, &procAttr)
	ch := make(chan struct{})

	go func() {
		process.Wait()
		debug("awin: process wait returned\n")
		close(ch)

		// There seems to be a bug on Windows where if the process terminates and
		// we were performing a non-blocking read, the read never returns. Also
		// it seems like runtime.Stack can't see that goroutine anymore either.
		// So we have a hack here to eventually exit after enough time that
		// all the process output has hopefully been read.
		time.Sleep(5 * time.Second)
		os.Exit(0)

		/*
			buf := make([]byte, 3000)
			sz := runtime.Stack(buf, true)
			buf = buf[0:sz]
			debug("awin: stack:\n%s\n", string(buf))
		*/
	}()

	terminated = func() bool {
		select {
		case <-ch:
			debug("awin: terminated: returning true\n")
			return true
		default:
		}
		debug("awin: terminated: returning false\n")
		return false
	}

	return
}

func clearLastLine(p *ProcessOutputHandler, clearLastLineChan chan struct{}) {
	for _ = range clearLastLineChan {
		p.clearLastLineFromProcess()
	}
}

func parseOpts() {
	pflag.Usage = usage
	pflag.Parse()
	doDebug = *optDebug
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> [argument...]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Run an interactive command-line process inside an Anvil window.\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")

	pflag.PrintDefaults()
}

func readNotifs(notifChan chan anvil.Notification) func(notif *anvil.Notification, err error) {
	return func(notif *anvil.Notification, err error) {
		if err != nil {
			debug("got an error handling notifications: %v\n", err)
			return
		}

		notifChan <- *notif
	}
}

type NotificationProcessor struct {
	lastLineFromProcess string
	cmdStdin            io.Writer
	notifChan           <-chan anvil.Notification
	lastLineChan        <-chan string
	clearLastLineChan   chan<- struct{}
	lastBodyEpoch       uint64
}

func NewNotificationProcessor(cmdStdin io.Writer, nc <-chan anvil.Notification,
	lastLineChan <-chan string, clearLastLineChan chan<- struct{}) NotificationProcessor {
	return NotificationProcessor{
		cmdStdin:          cmdStdin,
		notifChan:         nc,
		lastLineChan:      lastLineChan,
		clearLastLineChan: clearLastLineChan,
	}
}

func (p *NotificationProcessor) run() {
	for {
		select {
		case notif := <-p.notifChan:
			p.processExecNotif(notif)
			p.processBodyChangeNotif(notif)
		case l := <-p.lastLineChan:
			p.lastLineFromProcess = l
		}
	}
}

func (p *NotificationProcessor) processExecNotif(n anvil.Notification) {
	if n.Op == anvil.NotificationOpExec {
		if n.WinId != ttyWinId {
			return
		}

		if n.Cmd[0] != "Send" {
			return
		}

		p.processSendNotification(n)
	}
}

func (p *NotificationProcessor) processSendNotification(n anvil.Notification) {
	if len(n.Cmd) > 1 {
		cmd := strings.Join(n.Cmd[1:], " ")
		debug("awin: sending to process: '%s' (%v)\n", cmd, []byte(cmd))
		if ptyAllocated {
			fmt.Fprintf(p.cmdStdin, "%s\r", cmd)
		} else {
			fmt.Fprintf(p.cmdStdin, "%s\n", cmd)
		}
		return
	}
}

func (p *NotificationProcessor) processBodyChangeNotif(notif anvil.Notification) {
	var info anvil.WindowBody
	anvilHttpApi.GetInto(fmt.Sprintf("/wins/%d/body/info", ttyWinId), &info)

	isAppend := isNotifAnAppend(notif, info.Len)
	if !isAppend {
		return
	}

	rsp, err := anvilHttpApi.Get(fmt.Sprintf("/wins/%d/body", ttyWinId))
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))
	body, err := ioutil.ReadAll(rsp.Body)
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))

	if !notifOccurredAtEndOfBody(body, notif) {
		debug("notification is not for an append\n")
		return
	}

	debug("processing body changed notification: %+v\n", notif)

	pl := p.lastLineFromProcess
	l := promptOrLastFullLine(string(body))
	debug("lastLine: '%s'\n", l)
	if !endsWithByte(l, '\n') {
		return
	}

	if pl == l {
		return
	}

	l = stripPrompt(l, pl)

	debug("awin: sending to process: '%s' (%v)\n", l, []byte(l))
	fmt.Fprintf(p.cmdStdin, "%s", l)
	p.clearLastLineChan <- struct{}{}
}

func notifOccurredAtEndOfBody(body []byte, notif anvil.Notification) bool {
	return notif.Offset+notif.Len == len(body)
}

func readProcess(cmdStdout io.Reader, c chan<- []byte) {

	if conn, ok := cmdStdout.(syscall.Conn); ok {
		if rawConn, err := conn.SyscallConn(); err == nil {
			debug("awin: readProcess: using nonblocking read\n")
			readProcessNonblocking(rawConn, c)
			return
		} else {
			debug("awin: readProcess: stdout is a syscall.Conn, but SyscallConn returned error: %v. Falling back to blocking read\n", err)
		}
	} else {
		debug("awin: readProcess: stdout is not a syscall.Conn. Falling back to blocking read\n")
	}

	readProcessBlocking(cmdStdout, c)
}

func readProcessBlocking(cmdStdout io.Reader, c chan<- []byte) {
	buf := make([]byte, 100)

	for {
		n, err := cmdStdout.Read(buf)
		if err != nil {
			if err == io.EOF {
				debug("awin: Got EOF from process\n")
				break
			}

			if isTerminated() {
				debug("awin: process terminated\n")
				break
			}

			debug("awin: Read error: %v (%T). Will retry.\n", err, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		b := make([]byte, n)
		copy(b, buf[0:n])
		c <- b
	}

	close(c)

}

func readProcessNonblocking(conn syscall.RawConn, c chan<- []byte) {
	buf := make([]byte, 5000)
	var innerErr error
	read := func(fd uintptr) (done bool) {
		buf = buf[:5000]
		var n int
		n, innerErr = readFromFd(fd, buf)
		if innerErr == nil {
			buf = buf[:n]
		} else {
			debug("awin: nonblocking read read %d bytes on error\n", n)
		}
		return true
	}

	for {
		//debug("awin: doing nonblocking read\n")
		err := conn.Read(read)
		//debug("awin: done nonblocking read\n")

		if err != nil {
			debug("awin: nonblocking read returned error: %T\n", err)
			continue
		}

		if innerErr != nil {
			if err == io.EOF {
				debug("awin: Got EOF from process\n")
				break
			}

			if isTerminated() {
				debug("awin: process terminated\n")
				break
			}

			if err == nil {
				err = innerErr
			}
			debug("awin: Read error: %v (%T). Will retry.\n", err, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		b := make([]byte, len(buf))
		copy(b, buf)
		c <- b
	}

	close(c)
}

type ProcessOutputHandler struct {
	lastLine     bytes.Buffer
	lock         sync.Mutex
	procOutput   <-chan []byte
	lastLineChan chan<- string
	winId        int
	escParser    EscapeSequenceParser
}

func NewProcessOutputHandler(winId int, procOutput <-chan []byte, lastLineChan chan<- string) ProcessOutputHandler {
	return ProcessOutputHandler{
		winId:        winId,
		procOutput:   procOutput,
		lastLineChan: lastLineChan,
		escParser:    NewEscapeSequenceParser(),
	}
}

func (p *ProcessOutputHandler) run() {
	for buf := range p.procOutput {
		p.process(buf)
	}
}

func (p *ProcessOutputHandler) process(buf []byte) {
	debug("awin: output from process before cleaning as string: '%s'\n", visualizeEscapes(string(buf)))
	debug("awin: output from process before cleaning as hexdump:\n%s\n", hex.Dump(buf))

	cleaned := p.clean(buf)
	p.updateLastLineAndSendNotifs(cleaned)

	debug("awin: output from process after cleaning: '%s'\n", cleaned)
	debug("awin: last line from process: '%s'\n", lastLineFromProcess)
	p.appendText(cleaned)
	debug("awin: moving cursor to end of body\n")
	p.moveCursorToEndOfBody()
	debug("awin: done moving\n")
}

func (p *ProcessOutputHandler) clearLastLineFromProcess() {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.lastLine.Reset()
}

func (p *ProcessOutputHandler) updateLastLineAndSendNotifs(buf []byte) {
	for i, b := range buf {
		if b == '\n' && i < len(buf) {
			p.lastLine.Reset()
		} else {
			p.lastLine.WriteByte(b)
		}
	}

	lastLineFromProcess := p.lastLine.Bytes()
	p.lastLineChan <- string(lastLineFromProcess)

}

// stripControl atrips control characters, except for newline (linefeed)
func stripControl(buf []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, buf)
}

func (p *ProcessOutputHandler) clean(buf []byte) []byte {
	cleaned, _ := p.escParser.Input(buf)
	// cleaned points to an internal buffer that is overwritten each time Input is called.
	// We need to make a copy; ReplaceAll does that automatically
	cleaned = bytes.ReplaceAll(cleaned, []byte("\r\n"), []byte("\n"))

	cleaned = stripControl(cleaned)
	return cleaned
}

func (p *ProcessOutputHandler) appendText(buf []byte) {
	debug("awin: asked to append text '%s'\n", string(buf))

	if !bytes.ContainsRune(buf, '\r') {
		p.appendToWindowBody(buf)
		return
	}

	rsp, err := anvilHttpApi.Get(fmt.Sprintf("/wins/%d/body", p.winId))
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))
	body, err := ioutil.ReadAll(rsp.Body)
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))

	body = appendRespectingCRs(body, buf)

	abuf := bytes.NewBuffer(body)
	anvilHttpApi.Put(fmt.Sprintf("/wins/%d/body", p.winId), abuf)
}

func appendRespectingCRs(body, suffix []byte) []byte {
	startOfLineIndex := bytes.LastIndexByte(body, '\n') + 1
	bodyIndex := len(body)
	// Increase length of body
	body = append(body, suffix...)

	suffixIndex := 0
	for {
		if suffixIndex == len(suffix) {
			break
		}

		b := suffix[suffixIndex]
		if b == '\r' {
			bodyIndex = startOfLineIndex
			suffixIndex++
			continue
		}

		if b == '\n' {
			startOfLineIndex = bodyIndex + 1
		}

		body[bodyIndex] = b
		bodyIndex++
		suffixIndex++

	}
	body = body[:bodyIndex]

	return body
}

func (p *ProcessOutputHandler) appendToWindowBody(buf []byte) {
	r := bytes.NewReader(buf)
	anvilHttpApi.Post(fmt.Sprintf("/wins/%d/body", p.winId), r)
}

func (p *ProcessOutputHandler) moveCursorToEndOfBody() {
	var info anvil.WindowBody
	anvilHttpApi.GetInto(fmt.Sprintf("/wins/%d/body/info", p.winId), &info)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "[%d]", info.Len)
	anvilHttpApi.Put(fmt.Sprintf("/wins/%d/body/cursors", p.winId), &buf)
}

func endsWithByte(s string, b byte) bool {
	if s == "" {
		return false
	}

	return s[len(s)-1] == b
}

func stripPrompt(s, prompt string) string {
	if !endsWithByte(prompt, '\n') && strings.HasPrefix(s, prompt) {
		s = s[len(prompt):]
	}
	return s
}

func registerSendCommand(anvil *anvil.Anvil) {
	debug("awin: Registering Send command\n")
	var buf bytes.Buffer
	buf.WriteString(`["Send"]`)
	anvil.Post("/cmds", &buf)
	debug("awin: Done registering Send command\n")
}

func findOrCreateWindow(anvilHttpApi *anvil.Anvil, compoundPath string) anvil.Window {
	var wins []anvil.Window
	err := anvilHttpApi.GetInto("/wins", &wins)
	dieIfError(err, fmt.Sprintf("awin: "))
	for _, w := range wins {
		if w.Path == compoundPath {
			debug("awin: findOrCreateWindow: found existing window with path '%s' with winId %d\n", compoundPath, w.Id)
			return w
		}
	}

	win := createNewWindow(anvilHttpApi)
	setWindowTag(anvilHttpApi, win.Id, compoundPath)
	return win
}

func createNewWindow(anvilHttpApi *anvil.Anvil) anvil.Window {
	debug("awin: Creating new window\n")
	rsp, err := anvilHttpApi.Post("/wins", noBody)
	dieIfError(err, fmt.Sprintf("awin: "))
	debug("awin: Done creating new window\n")

	raw, err := ioutil.ReadAll(rsp.Body)
	dieIfError(err, fmt.Sprintf("awin: Error reading response body in POST to /wins"))

	var win anvil.Window
	err = json.Unmarshal(raw, &win)
	dieIfError(err, fmt.Sprintf("awin: Error decoding JSON response body in POST to /wins"))
	debug("New window id: %d\n", win.Id)
	return win
}

func setWindowTag(anvilHttpApi *anvil.Anvil, winId int, compoundPath string) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s Del! Snarf | Look  Send ", compoundPath)
	anvilHttpApi.Put(fmt.Sprintf("/wins/%d/tag", winId), &buf)
}

func compoundPathForTag(winPath string, argv []string) string {
	cmd := ""
	if len(argv) > 0 {
		cmd = argv[0]
	}

	return fmt.Sprintf("%s-%s", winPath, cmd)
}

var lastLineFromProcess string
var lock sync.Mutex

func doNotifsContainAnAppend(notifs []anvil.Notification, bodyLen int) bool {
	for _, notif := range notifs {
		if notif.WinId != ttyWinId {
			continue
		}

		if notif.Op == anvil.NotificationOpInsert && notif.Offset+notif.Len == bodyLen {
			return true
		}
	}

	return false
}

func isNotifAnAppend(notif anvil.Notification, bodyLen int) bool {
	if notif.WinId != ttyWinId {
		return false
	}

	if notif.Op == anvil.NotificationOpInsert && notif.Offset+notif.Len == bodyLen {
		return true
	}
	return false
}

func getEnvOrDie(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "awin: Environment variable %s is not set\n", name)
		os.Exit(1)
	}

	return v
}

func dieIfError(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "awin: %s: %s\n", msg, err)
		os.Exit(1)
	}
}

func promptOrLastFullLine(s string) string {
	if s == "" {
		return ""
	}

	if l := len(s) - 1; s[l] == '\n' {
		return textAfterLastNewline(s[:l]) + "\n"
	}
	return textAfterLastNewline(s)
}

func textAfterLastNewline(s string) string {
	if len(s) == 0 || s[len(s)-1] == '\n' {
		return ""
	}

	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return s
}

func commandAndArgsToRun() (argv []string, err error) {
	if len(pflag.Args()) < 1 {
		err = fmt.Errorf("No command specified")
		return
	}

	argv = pflag.Args()
	return
}

func visualizeEscapes(s string) string {
	var buf bytes.Buffer

	for _, r := range s {
		if r < 0x20 {
			switch r {
			case 0x07:
				buf.WriteString(`\a`)
			case 0x08:
				buf.WriteString(`\b`)
			case 0x09:
				buf.WriteString(`\t`)
			case 0x0d:
				buf.WriteString(`\r`)
			case 0x0a:
				buf.WriteString(`\n`)
			case 0x0b:
				buf.WriteString(`\t`)
			case 0x1b:
				buf.WriteString(`\e`)
			default:
				fmt.Fprintf(&buf, `\x%x`, r)
			}
			continue
		}

		buf.WriteRune(r)
	}
	return buf.String()
}
