package main

import (
	"bytes"
	"container/list"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/jeffwilliams/terminal"
	"github.com/ogier/pflag"
)

var (
	cmdArgv                           []string
	anvilHttpApi                      anvil.Anvil
	anvilWsApi                        anvil.Websock
	win                               anvil.Window
	doDebug                           = true
	termState                         *terminal.State
	vTerm                             *terminal.VT
	winWidthInRunes, winHeightInRunes int
	attached                          bool
	cmd                               *exec.Cmd
	killed                            bool
	exiting                           bool
	scrollback                        *list.List
	maxScrollbackLines                = 500
	restartChan                       chan struct{}
)

var (
	optDebug          = pflag.BoolP("debug", "d", false, "Print debug messages")
	optWinPath        = pflag.StringP("win-path", "n", "", "Window path")
	optUseExistingWin = pflag.BoolP("win", "w", false, "Rather than making a new window, use the window in which this command was run.")
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

	var err error
	cmdArgv, err = commandAndArgsToRun()
	if err != nil {
		usage()
		os.Exit(1)
	}

	setupAnvil()

	scrollback = list.New()
	restartChan = make(chan struct{})

	origStdin, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		origStdin = nil
	}

	for {
		termState, vTerm, _, cmd, err = startCmd(cmdArgv)
		setTermSizeToWindowSize()
		termState.OnScroll(textScrolled)
		dieIfError(err, fmt.Sprintf("awin: Starting command failed: %v\n", err))

		if origStdin != nil && len(origStdin) > 0 {
			vTerm.File().Write(origStdin)
		}

		processTermChanges(vTerm)
		adjustTagRestart(false)
		_ = <-restartChan
		adjustTagRestart(true)
	}
}

func setupAnvil() {
	var err error

	anvilGlobalPath := os.Getenv("ANVIL_WIN_GLOBAL_PATH")
	if anvilGlobalPath == "" {
		var err error
		anvilGlobalPath, err = os.Getwd()
		dieIfError(err, fmt.Sprintf("awin: Environment variable ANVIL_WIN_GLOBAL_PATH is not set and getting current dir failed"))
	}

	anvilHttpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handleNotification,
	}

	anvilWsApi, err = anvilHttpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")

	err = registerCommands()
	dieIfError(err, "registering commands failed")

	compoundPath := *optWinPath
	if compoundPath == "" {
		compoundPath = compoundPathForTag(anvilGlobalPath, cmdArgv)
	}
	win = findOrCreateWindow(&anvilHttpApi, compoundPath)

	attach()

	go anvilWsApi.Run()
}

func attach() {
	err := anvilHttpApi.RegisterForWindowNotification(win, anvil.NotificationOpKeyPress)
	printError(err, "registering for keypress notifications failed")
	err = anvilHttpApi.RegisterForWindowNotification(win, anvil.NotificationOpTextInput)
	printError(err, "registering for text input notifications failed")
	if err != nil {
		return
	}
	adjustTagAttach(true)
}

func detach() {
	err := anvilHttpApi.UnregisterForWindowNotification(win, anvil.NotificationOpKeyPress)
	printError(err, "unregistering for keypress notifications failed")
	err = anvilHttpApi.UnregisterForWindowNotification(win, anvil.NotificationOpTextInput)
	printError(err, "unregistering for text input notifications failed")
	if err != nil {
		return
	}
	adjustTagAttach(false)
}

func processTermChanges(vt *terminal.VT) {
	for {
		err := vt.Parse()
		if err != nil {
			debug("Got an error from vt.Parse(): %v\n", err)
			vt.Write([]byte("(terminated)"))
			if killed {
				break
			}
			redrawWindow()
			break
		}
		debug("Redrawing window\n")
		redrawWindow()
	}
	stopTerm(vt)
}

func stopTerm(vt *terminal.VT) {
	vt.File().Close()
	vt.Close()
}

func redrawWindow() {
	conv := NewConverter(termState)
	conv.Convert(winWidthInRunes, winHeightInRunes)
	err := anvilHttpApi.SetWindowBodyLeaveCursors(win, &conv.bodyText)
	printError(err, "error setting window body")
	err = anvilHttpApi.SetWindowBodyCursors(win, []int{conv.cursorPos})
	printError(err, "error putting cursors")
	err = anvilHttpApi.SetWindowBodyTints(win, conv.tints)
	printError(err, "error setting window tints")
}

var ptyAllocated bool

func startCmd(argv []string) (state *terminal.State, vt *terminal.VT, pty *os.File, cmd *exec.Cmd, err error) {
	cmd = exec.Command(argv[0], argv[1:]...)
	cmd.Env = buildEnv()
	state = &terminal.State{}
	vt, pty, err = terminal.Start(state, cmd)
	return
}

func buildEnv() []string {
	env := os.Environ()
	termFound := false
	for _, v := range env {
		if strings.HasPrefix(v, "TERM=") {
			termFound = true
			break
		}
	}
	if !termFound {
		env = append(env, "TERM=xterm")
	}
	return env
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

func registerCommands() error {
	return anvilHttpApi.RegisterCommands("Detach", "Attach", "Send", "Sendln", "Hist", "Restart", "Quit")
}

func handleNotification(notif *anvil.Notification, err error) {
	if err != nil {
		printError(err, "got an error handling notifications")
		return
	}

	switch notif.Op {
	case anvil.NotificationOpKeyPress:
		handleKeyPressNotification(notif)
	case anvil.NotificationOpTextInput:
		handleTextInputNotification(notif)
	case anvil.NotificationOpWinSizeChanged:
		handleWinSizeChangedNotification(notif)
	case anvil.NotificationOpExec:
		handleExecNotification(notif)
		//	case anvil.NotificationOpFileClosed:
		//		// This was not always working because there is only a notification from
		//		// Anvil when the last window with the name is deleted.
		//		handleFileClosedNotification(notif)
	case anvil.NotificationOpWinClosed:
		handleWinClosedNotification(notif)

	}

}

func textScrolled(l terminal.Line) {
	if scrollback.Len() >= maxScrollbackLines {
		scrollback.Front().Value = textOfLine(l)
		scrollback.MoveToBack(scrollback.Front())
		return
	}

	scrollback.PushBack(textOfLine(l))
	//fmt.Printf("scrollback: '%s'\n", textOfLine(l))
}

func printScrollback() {
	for e := scrollback.Front(); e != nil; e = e.Next() {
		fmt.Printf("%s\n", e.Value.(string))
	}

	var line bytes.Buffer
	for y := range winHeightInRunes {
		line.Reset()
		for x := range winWidthInRunes {
			rn, _, _ := termState.Cell(x, y)
			line.WriteRune(rn)
		}
		os.Stdout.Write(line.Bytes())
		os.Stdout.Write([]byte{'\n'})
	}
}

var textOfLineBuf bytes.Buffer

func textOfLine(l terminal.Line) string {
	textOfLineBuf.Reset()
	for i := range l.Len() {
		r, _, _ := l.Cell(i)
		textOfLineBuf.WriteRune(r)
	}
	return textOfLineBuf.String()
}

type KeyModifiers int

const (
	ModCtrl KeyModifiers = 1 << iota
	ModCommand
	ModShift
	ModAlt
	ModSuper
)

var oneByteSlice = []byte{'X'}

// handleKeyPressNotification translates keypresses that Anvil sends into equivalent terminal escape codes or characters and sends them to the terminal.
//
// Copy, Cut and Paste is implemented using the IBM Common User Access standards - inspired by first answer in https://unix.stackexchange.com/questions/114392/copy-paste-in-a-terminal-without-shift
//
func handleKeyPressNotification(n *anvil.Notification) {
	if n.WinId != win.Id {
		return
	}

	put := func(s string) {
		vTerm.File().Write([]byte(s))
	}

	putc := func(c byte) {
		oneByteSlice[0] = c
		vTerm.File().Write(oneByteSlice)
	}

	keyModifiers := KeyModifiers(n.Offset)
	keyName := n.Cmd[0]

	switch keyName {
	case "⏎":
		// Enter
		putc('\n')
	case "⎋":
		// Escape
		putc('\x1b')
	case "⌫":
		// Backspace
		putc(ansi.BS)
	case "⌦":
		// Delete
		if keyModifiers&ModShift > 0 {
			err := anvilHttpApi.Execute("Cut", []string{})
			printError(err, "sending Cut command to anvil failed")
			break
		}
		putc(ansi.ESC)
		put("[3~")
	case "Insert":
		if keyModifiers&ModCtrl > 0 {
			err := anvilHttpApi.Execute("Snarf", []string{})
			printError(err, "sending Copy command to anvil failed")
		} else if keyModifiers&ModShift > 0 {
			err := anvilHttpApi.Execute("Paste", []string{})
			printError(err, "sending Copy command to anvil failed")
		}
	case "Shift", "Ctrl", "Alt":
		// Ignore. Send it back so Anvil processes it
		err := anvilHttpApi.WindowBodyKeypress(win, keyName, int(keyModifiers))
		printError(err, "sending keypress to anvil failed")
	case "Tab":
		putc('\t')
	case "←":
		if keyModifiers&ModCtrl > 0 {
			putc(ansi.ESC)
			put("[1;5D")
			break
		}
		put(ansi.CUB1)
	case "→":
		if keyModifiers&ModCtrl > 0 {
			putc(ansi.ESC)
			put("[1;5C")
			break
		}
		put(ansi.CUF1)
	case "↑":
		put(ansi.CUU1)
	case "↓":
		put(ansi.CUD1)
	case "⇟":
		// Page down
		putc(ansi.ESC)
		put("[6~")
	case "⇞":
		// Page up
		putc(ansi.ESC)
		put("[5~")
	case "⇱":
		// Home
		putc(ansi.ESC)
		put("[7~")
	case "⇲":
		// End
		putc(ansi.ESC)
		put("[8~")
	default:
		if keyModifiers&ModCtrl > 0 {
			switch {
			case keyName == "C":
				putc(ansi.ETX) // CTRL-C == ETX
			case keyName == "A":
				putc(ansi.SOH)
			case keyName == "E":
				putc(ansi.ENQ)
			case keyName == "B":
				putc(ansi.STX)
			case keyName == "D":
				putc(ansi.EOT)
			case keyName == "F":
				putc(ansi.ACK)
			case keyName == "G":
				putc(ansi.BEL)
			case keyName == "H":
				putc(ansi.BS)
			case keyName == "@":
				putc(ansi.NUL)
			case keyName == "I":
				putc(ansi.HT)
			case keyName == "J":
				putc(ansi.LF)
			case keyName == "K":
				putc(ansi.VT)
			case keyName == "L":
				putc(ansi.FF)
			case keyName == "M":
				putc(ansi.CR)
			case keyName == "N":
				putc(ansi.SO)
			case keyName == "O":
				putc(ansi.SI)
			case keyName == "P":
				putc(ansi.DLE)
			case keyName == "Q":
				putc(ansi.DC1)
			case keyName == "R":
				putc(ansi.DC2)
			case keyName == "S":
				putc(ansi.DC3)
			case keyName == "T":
				putc(ansi.DC4)
			case keyName == "U":
				putc(ansi.NAK)
			case keyName == "V":
				putc(ansi.SYN)
			case keyName == "W":
				putc(ansi.ETB)
			case keyName == "X":
				putc(ansi.CAN)
			case keyName == "Y":
				putc(ansi.EM)
			case keyName == "Z":
				putc(ansi.SUB)
			case keyName == "[":
				putc(ansi.ESC)
			case keyName == "\\":
				putc(ansi.FS)
			case keyName == "]":
				putc(ansi.GS)
			case keyName == "^":
				putc(ansi.RS)
			case keyName == "_":
				putc(ansi.US)
			}
			break
		}
	}
}

func handleTextInputNotification(n *anvil.Notification) {
	if n.WinId != win.Id {
		return
	}

	text := n.Cmd[0]
	vTerm.File().Write([]byte(text))
}

func handleWinSizeChangedNotification(n *anvil.Notification) {
	if n.WinId != win.Id {
		return
	}

	setTermSizeToWindowSize()
}

func setTermSizeToWindowSize() {
	var err error
	winWidthInRunes, winHeightInRunes, err = windowSizeInRunes(win)
	debug("getting window size in runes failed: %v\n", err)
	if vTerm != nil {
		vTerm.Resize(winWidthInRunes, winHeightInRunes)
	}
}

func handleExecNotification(n *anvil.Notification) {
	if n.WinId != win.Id {
		return
	}

	switch n.Cmd[0] {
	case "Attach":
		attach()
	case "Detach":
		detach()
	case "Send":
		vTerm.File().Write([]byte(strings.Join(n.Cmd[1:], " ")))
	case "Sendln":
		s := strings.Join(n.Cmd[1:], " ")
		if len(s) == 0 {
			s = "\n"
		} else if s[len(s)-1] != '\n' {
			s = s + "\n"
		}
		vTerm.File().Write([]byte(s))
	case "Hist":
		printScrollback()
	case "Restart":
		select {
		case restartChan <- struct{}{}:
		default:
		}
	case "Quit":
		kill()
	}
}

func handleWinClosedNotification(n *anvil.Notification) {
	if n.WinId != win.Id {
		return
	}

	debug("Got delete notification for window id %d\n", n.WinId)

	if exiting {
		return
	}
	exiting = true

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		killed = true
	}

	debug("exiting\n")
	os.Exit(0)
}

func kill() {
	if exiting {
		return
	}
	exiting = true

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		killed = true
	}

	err := anvilHttpApi.ExecuteInWin(win, "Del!", nil)
	if err != nil {
		fmt.Printf("Deleting window failed: %v\n", err)
	}

	os.Exit(0)
}

func findOrCreateWindow(anvilHttpApi *anvil.Anvil, compoundPath string) anvil.Window {
	//	wins, err := anvilHttpApi.Windows()
	//	dieIfError(err, fmt.Sprintf("awin: "))
	//	for _, w := range wins {
	//		debug("awin: findOrCreateWindow: compare '%s' to '%s'\n", w.Path,compoundPath)
	//		if w.Path == compoundPath {
	//			debug("awin: findOrCreateWindow: found existing window with path '%s' with winId %d\n", compoundPath, w.Id)
	//			return w
	//		}
	//	}

	if *optUseExistingWin {
		id := os.Getenv("ANVIL_WIN_ID")
		die("environment variable ANVIL_WIN_ID is not set, and I was requested to use the existing window")
		i, err := strconv.Atoi(id)
		dieIfError(err, "cannot convert value of environment variable ANVIL_WIN_ID to int")
		win, err := anvilHttpApi.Window(i)
		dieIfError(err, "getting window failed")
		return win
	}

	win := createNewWindow(anvilHttpApi)
	setWindowTag(anvilHttpApi, win.Id, compoundPath)
	return win
}

func createNewWindow(anvilHttpApi *anvil.Anvil) anvil.Window {
	debug("awin: Creating new window\n")
	win, err := anvilHttpApi.NewWindow()
	dieIfError(err, "creating window failed")
	debug("awin: Done creating new window\n")

	// Use the mono font
	anvilHttpApi.ExecuteInWin(win, "Font", []string{})

	// Pin it
	anvilHttpApi.ExecuteInWin(win, "Pin", []string{})

	return win
}

func setWindowTag(anvilHttpApi *anvil.Anvil, winId int, compoundPath string) {
	var buf bytes.Buffer
	//fmt.Fprintf(&buf, "%s Del! Snarf | Detach Quit Hist Send ", compoundPath)
	fmt.Fprintf(&buf, "%s Del! Snarf | Detach Hist Send Sendln", compoundPath)
	anvilHttpApi.Put(fmt.Sprintf("/wins/%d/tag", winId), &buf)
}

func windowSizeInRunes(win anvil.Window) (w, h int, err error) {
	body, err := anvilHttpApi.WindowBodyInfo(win)
	if err != nil {
		return 0, 0, err
	}
	if body.WidthInRunes <= 0 || body.HeightInRunes <= 0 {
		return 0, 0, fmt.Errorf("window width or height in runes is <= 0")
	}

	return body.WidthInRunes, body.HeightInRunes, nil
}

func compoundPathForTag(winPath string, argv []string) string {
	cmd := ""
	if len(argv) > 0 {
		cmd = argv[0]
	}

	return fmt.Sprintf("%s-%s", winPath, cmd)
}

func dieIfError(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "aterm: %s: %s\n", msg, err)
		os.Exit(1)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "aterm: %s\n", msg)
	os.Exit(1)
}

func printError(err error, msg string) {
	if err != nil {
		fmt.Printf("aterm: %s: %s\n", msg, err)
	}
}

func commandAndArgsToRun() (argv []string, err error) {
	if len(pflag.Args()) < 1 {
		err = fmt.Errorf("No command specified")
		return
	}

	argv = pflag.Args()
	return
}
