package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"
	rpc "go.lsp.dev/jsonrpc2"
)

// "github.com/urfave/cli/v3"

var (
	optDebug bool
)

func main() {
	cmd := &cli.Command{
		Name:  "lspd",
		Usage: "use an LSP from Anvil",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("debug") {
				optDebug = true
			}
			workspace := cmd.String("workspace")
			if workspace == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory failed %v\n", err)
				}
				workspace = wd
			}

			var err error
			lspConn, err = initLspServer(cmd) //Lsp{conn}
			dieIfError(err, "Starting LSP server failed")
			runAnvilServer(workspace)
			return nil
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Print debug info",
			},
			&cli.StringFlag{
				Name:    "clang-log-level",
				Aliases: []string{"l"},
				Usage:   "Clang log level. Should be one of 'error', 'info', or 'verbose'",
			},
			&cli.StringFlag{
				Name:    "workspace",
				Aliases: []string{"w"},
				Usage:   "Root directory of workspace",
			},
			&cli.StringFlag{
				Name:    "lsp",
				Aliases: []string{"s"},
				Usage:   "Name of the lsp to start. One of clangd, gopls.",
			},
			&cli.StringFlag{
				Name:    "lsp-path",
				Aliases: []string{"c"},
				Usage:   "Full path to the clangd binary to execute",
			},
			&cli.StringFlag{
				Name:    "compile-commands-dir",
				Aliases: []string{"m"},
				Usage:   "Path to the directory containing compile-commands.json",
			},
		},
		Commands: []*cli.Command{
			{
				Name:      "gen",
				Usage:     "Generate clangd compilation database file. The directory arguments to the command are scanned recursively and the .c and .cpp files are added to the compile_commands.json file.",
				UsageText: "aclangd gen [options] [dir...]",
				Action:    cliCommandGen,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "workspace-root",
						Aliases: []string{"r"},
						Value:   "",
						Usage:   "Directory in which to write the compile_commands.json. If not specified, it's put in the same place as the argument",
					},
					&cli.StringFlag{
						Name:    "build-dir",
						Aliases: []string{"b"},
						Value:   "",
						Usage:   "Directory in which the compile commands are executed. If not specified, it's put in the same place as the argument",
					},
					&cli.StringFlag{
						Name:    "compiler-args",
						Aliases: []string{"a"},
						Value:   "",
						Usage:   "Extra arguments to insert into the clang compilation command",
					},
				},
			},
			{
				Name:   "demo",
				Usage:  "Just a demo",
				Action: cliCommandDemo,
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cliCommandGen(ctx context.Context, cmd *cli.Command) error {
	workspaceRoot := cmd.String("workspace-root")
	extraArgs := cmd.String("compiler-args")
	buildDir := cmd.String("build-dir")

	//dirs := cmd.StringSlice("dirs")

	if cmd.NArg() == 0 {
		return fmt.Errorf("error: `aclang gen` requires a directory to scan as the argument\n")
	}

	dirs := cmd.Args().Slice()
	fmt.Printf("num dirs: %d\n", len(dirs))

	if workspaceRoot == "" {
		workspaceRoot = dirs[0]
	}

	if buildDir == "" {
		buildDir = dirs[0]
	}

	g, err := NewDbGenerator(workspaceRoot, buildDir)
	if err != nil {
		return err
	}
	g.extraArgs = extraArgs

	for _, d := range dirs {
		err = g.Add(d)
		if err != nil {
			return err
		}
	}

	return nil
}

func initLspServer(cmd *cli.Command) (lsp Lsp, err error) {
	lspName := cmd.String("lsp")
	if lspName == "" {
		err = fmt.Errorf("The --lsp argument is mandatory\n")
		return
	}

	var cmdOpts lspCmdOpts

	if s := cmd.String("lsp-path"); s != "" {
		cmdOpts.lspCmdPath = s
	}

	switch lspName {
	case "clangd":
		return initClangdUsingCliFlags(cmdOpts, cmd)
	case "gopls":
		return initGoplsUsingCliFlags(cmdOpts, cmd)
	default:
		err = fmt.Errorf("Unknown lsp %s\n", lspName)
		return
	}

	return
	//	level := clangLogLevelError
	//	if optDebug {
	//		level = clangLogLevelVerbose
	//	}
	//	s := cmd.String("clang-log-level")
	//	switch s {
	//	case "verbose":
	//		level = clangLogLevelVerbose
	//	case "error":
	//		level = clangLogLevelError
	//	case "info":
	//		level = clangLogLevelInfo
	//	}
	//	debug("Setting clang log level to %s\n", level)
	//
	// opts := newClangdOpts()
	//
	//	opts.logLevel = level
	//	if s = cmd.String("compile-commands-dir"); s != "" {
	//		opts.compileCommandsDir = s
	//	}
	//
	//	conn, err := initClangd(opts)
	//	lspConn = Lsp{conn}
	//	dieIfError(err, "Starting LSP server failed")
}

func initClangdUsingCliFlags(cmdOpts lspCmdOpts, cmd *cli.Command) (lsp Lsp, err error) {
	if cmdOpts.lspCmdPath == "" {
		cmdOpts.lspCmdPath = "clangd"
	}

	level := clangLogLevelError
	if optDebug {
		level = clangLogLevelVerbose
	}
	s := cmd.String("clang-log-level")
	switch s {
	case "verbose":
		level = clangLogLevelVerbose
	case "error":
		level = clangLogLevelError
	case "info":
		level = clangLogLevelInfo
	}
	debug("Setting clang log level to %s\n", level)
	opts := newClangdOpts()
	opts.logLevel = level
	if s = cmd.String("compile-commands-dir"); s != "" {
		opts.compileCommandsDir = s
	}

	conn, err := initClangd(cmdOpts, opts)
	lsp = Lsp{conn}
	return
}

func initGoplsUsingCliFlags(cmdOpts lspCmdOpts, cmd *cli.Command) (lsp Lsp, err error) {
	if cmdOpts.lspCmdPath == "" {
		cmdOpts.lspCmdPath = "gopls"
	}

	conn, err := initGopls(cmdOpts)
	lsp = Lsp{conn}
	return
}

func runAnvilServer(workspace string) {
	connectToAnvil()
	err := anvilHttpApi.RegisterCommands("Lhelp", "Ldef", "Ldecl", "Lrefs", "Lhov")
	dieIfError(err, "Registering Anvil command failed")

	//	conn, err := initClangd(opts)
	//	lspConn = Lsp{conn}
	//	dieIfError(err, "Starting LSP server failed")

	err = lspConn.Init(workspace)
	dieIfError(err, "Initializing LSP server failed")

	notifyOpenAnvilDocs()

	printHelp()

	anvilWsApi.Run()
}

func cliCommandDemo(ctx context.Context, cmd *cli.Command) error {

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getting working directory failed %v\n", err)
		os.Exit(1)
	}

	client, err := initClangd(lspCmdOpts{}, newClangdOpts())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	initMsg := Initialize{
		ProcessId: os.Getpid(),
		RootUri:   fmt.Sprintf("file://%s", wd),
		Capabilities: ClientCapabilities{
			OffsetEncoding: []string{"utf-8"},
			General: GeneralClientCapabilities{
				PositionEncodings: []string{"utf-8"},
			},
		},
	}

	var initRspMsg InitializeResult

	_, err = client.Call(context.Background(), "initialize", initMsg, &initRspMsg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: msg 'initialize' failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%#v\n", initRspMsg)

	err = client.Notify(context.Background(), "initialized", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: msg 'initialized' failed: %v\n", err)
		os.Exit(1)
	}

	data, err := ioutil.ReadFile("example-file.c")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading file example-file.c failed: %v\n", err)
		os.Exit(1)
	}

	docNotif := DidOpenTextDocument{
		TextDocument: TextDocumentItem{
			Uri:        fmt.Sprintf("file:///home/jefwill3/src/anvil-suite/anvil-extras/cmd/aclangd/sample/example-file.c"),
			LanguageId: "c",
			Version:    1,
			Text:       string(data),
		},
	}
	err = client.Notify(context.Background(), "textDocument/didOpen", docNotif)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: msg 'initialized' failed: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(2 * time.Second)

	getDefinition := TextDocumentPosition{
		TextDocumentIdentifier{
			Uri: fmt.Sprintf("file:///home/jefwill3/src/anvil-suite/anvil-extras/cmd/aclangd/sample/example-file.c"),
		},
		Position{
			Line:      7,
			Character: 4,
		},
	}

	var loc []Location

	_, err = client.Call(context.Background(), "textDocument/definition", getDefinition, &loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: msg 'initialize' failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%#v\n", loc)

	for {
		time.Sleep(1 * time.Second)
	}
}

func handler(ctx context.Context, reply rpc.Replier, req rpc.Request) error {
	switch req.Method() {
	case "textDocument/publishDiagnostics":
		raw := req.Params()
		var msg PublishDiagnosticParams
		err := json.Unmarshal(raw, &msg)
		if err == nil {
			p := uriToPath(msg.Uri)
			printMsg("diagnostics for %s:\n", p)
			for _, d := range msg.Diagnostics {
				printMsg("  %s:%d:%d: %s\n", filepath.Base(p), d.Range.Start.Line, d.Range.Start.Character, d.Message)
			}
		}
	default:
		printMsg("handler: got unimplemented request %s\n", req.Method())
	}
	return nil
}

type ReadWriter struct {
	reader io.ReadCloser
	writer io.WriteCloser
}

func (rw ReadWriter) Read(p []byte) (n int, err error) {
	return rw.reader.Read(p)
}

func (rw ReadWriter) Write(p []byte) (n int, err error) {
	return rw.writer.Write(p)
}

func (rw ReadWriter) Close() error {
	return rw.writer.Close()
}

type LoggingWriter struct {
	writer io.Writer
}

func (w LoggingWriter) Write(p []byte) (n int, err error) {
	fmt.Fprintf(os.Stderr, "%s", string(p))
	return w.writer.Write(p)
}

func (w LoggingWriter) Close() error {
	if c, ok := w.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type LoggingReader struct {
	reader io.Reader
}

func (r LoggingReader) Read(p []byte) (n int, err error) {
	//fmt.Printf("read of size %d\n", len(p))
	n, err = r.reader.Read(p)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s", string(p))
	return
}

func (r LoggingReader) Close() error {
	if c, ok := r.reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
