package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	rpc "go.lsp.dev/jsonrpc2"
)

var (
	lspConn Lsp
)

type Lsp struct {
	lspConn rpc.Conn
}

func (l Lsp) Init(wsRoot string) (err error) {
	initMsg := Initialize{
		ProcessId: os.Getpid(),
		RootUri:   fmt.Sprintf("file://%s", wsRoot),
		Capabilities: ClientCapabilities{
			OffsetEncoding: []string{"utf-8"},
			General: GeneralClientCapabilities{
				PositionEncodings: []string{"utf-8"},
			},
		},
	}

	var initRspMsg InitializeResult

	_, err = l.lspConn.Call(context.Background(), "initialize", initMsg, &initRspMsg)
	if err != nil {
		err = fmt.Errorf("error: lsp 'initialize' failed: %v", err)
	}

	err = l.lspConn.Notify(context.Background(), "initialized", nil)
	if err != nil {
		err = fmt.Errorf("error: lsp 'initialized' failed: %v", err)
	}

	return
}

func (l Lsp) DocOpened(path string, contents string) (err error) {
	docNotif := DidOpenTextDocument{
		TextDocument: TextDocumentItem{
			Uri:        fmt.Sprintf("file://%s", path),
			LanguageId: "c",
			Version:    1,
			Text:       contents,
		},
	}

	err = l.lspConn.Notify(context.Background(), "textDocument/didOpen", docNotif)
	if err != nil {
		err = fmt.Errorf("error: lsp 'textDocument/didOpen' failed: %v", err)
	}
	return
}

func (l Lsp) DocSaved(path string) (err error) {
	docNotif := DidSaveTextDocument{
		TextDocument: TextDocumentIdentifier{
			Uri: fmt.Sprintf("file://%s", path),
		},
	}

	err = l.lspConn.Notify(context.Background(), "textDocument/didSave", docNotif)
	if err != nil {
		err = fmt.Errorf("error: lsp 'textDocument/didSave' failed: %v", err)
	}
	return

}

// GetDefinition dets the definition of the identifier at the specified line and char in the file `path`.
// line and char are 1-based; they are translated to 0-based
func (l Lsp) GetDefinition(path string, line, char uint) (simple []SimpleLocation, err error) {
	return l.getLocation("textDocument/definition", path, line, char)
}

func (l Lsp) GetDeclaration(path string, line, char uint) (simple []SimpleLocation, err error) {
	return l.getLocation("textDocument/declaration", path, line, char)
}

func (l Lsp) GetReferences(path string, line, char uint) (simple []SimpleLocation, err error) {
	return l.getLocation("textDocument/references", path, line, char)
}

// getLocation performs a request that contains a file identifier and cursor position, and returns a location.
func (l Lsp) getLocation(method string, path string, line, char uint) (simple []SimpleLocation, err error) {
	var getDeclaration TextDocumentPosition
	getDeclaration, err = LspDocPositionFromAnvilPosition(path, line, char)
	if err != nil {
		return
	}

	var loc []Location
	_, err = l.lspConn.Call(context.Background(), method, getDeclaration, &loc)
	if err != nil {
		err = fmt.Errorf("error: msg '%s' failed: %v", method, err)
	}

	simple = SimpleLocationsFromLocations(loc)
	return

}

func LspDocPositionFromAnvilPosition(path string, line, char uint) (pos TextDocumentPosition, err error) {
	line--
	char--
	path, err = filepath.Abs(path)
	if err != nil {
		err = fmt.Errorf("error: getting absolute path failed: %v", err)
	}

	pos = TextDocumentPosition{
		TextDocumentIdentifier{
			Uri: fmt.Sprintf("file://%s", path),
		},
		Position{
			Line:      line,
			Character: char,
		},
	}
	return
}

type SimpleLocation struct {
	Path  string
	Range Range
}

func SimpleLocationsFromLocations(loc []Location) []SimpleLocation {
	simple := make([]SimpleLocation, len(loc))

	for i, e := range loc {
		simple[i].Path = uriToPath(e.Uri)
		simple[i].Range.Start.Line = e.Range.Start.Line + 1
		simple[i].Range.Start.Character = e.Range.Start.Character + 1
		simple[i].Range.End.Line = e.Range.End.Line + 1
		simple[i].Range.End.Character = e.Range.End.Character + 1
	}
	return simple
}

func (l Lsp) Hover(path string, line, char uint) (info string, err error) {
	var req TextDocumentPosition
	req, err = LspDocPositionFromAnvilPosition(path, line, char)
	if err != nil {
		return
	}

	var rsp HoverResponse
	_, err = l.lspConn.Call(context.Background(), "textDocument/hover", req, &rsp)
	if err != nil {
		err = fmt.Errorf("error: msg '%s' failed: %v", "textDocument/hover", err)
	}

	info = rsp.Contents
	return
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return uri[7:]
	}
	return uri
}

type clangLogLevel string

const (
	clangLogLevelError   clangLogLevel = "error"
	clangLogLevelInfo    clangLogLevel = "info"
	clangLogLevelVerbose clangLogLevel = "verbose"
)

type lspCmdOpts struct {
	lspCmdPath string
}

type clangdOpts struct {
	clangdPath         string
	compileCommandsDir string
	logLevel           clangLogLevel
}

func newClangdOpts() clangdOpts {
	return clangdOpts{
		clangdPath: "clangd",
		logLevel:   clangLogLevelError,
	}
}

func initClangd(cmdOpts lspCmdOpts, clangOpts clangdOpts) (client rpc.Conn, err error) {
	log := fmt.Sprintf("--log=%s", clangOpts.logLevel)

	args := []string{log, "--background-index"}
	if clangOpts.compileCommandsDir != "" {
		args = append(args, fmt.Sprintf("--compile-commands-dir=%s", clangOpts.compileCommandsDir))
	}
	return initLsp("clangd", cmdOpts.lspCmdPath, args...)

	//	cmd := exec.Command(opts.clangdPath, args...)
	//
	//	stdin, err := cmd.StdinPipe()
	//	if err != nil {
	//		err = fmt.Errorf("creating stdin pipe to clangd failed: %v", err)
	//		return
	//	}
	//
	//	stdout, err := cmd.StdoutPipe()
	//	if err != nil {
	//		err = fmt.Errorf("creating stdin pipe to clangd failed: %v", err)
	//		return
	//	}
	//
	//	cmd.Stderr = os.Stderr
	//
	//	err = cmd.Start()
	//	if err != nil {
	//		err = fmt.Errorf("starting clangd failed: %v", err)
	//		return
	//	}
	//
	//	rw := ReadWriter{stdout, stdin}
	//	stream := rpc.NewStream(rw)
	//	client = rpc.NewConn(stream)
	//
	//	client.Go(context.Background(), handler)
	//
	//	return

}

func initGopls(opts lspCmdOpts) (client rpc.Conn, err error) {
	return initLsp("gopls", opts.lspCmdPath)
}

func initLsp(friendlyLspCmdName, cmdPath string, args ...string) (client rpc.Conn, err error) {
	cmd := exec.Command(cmdPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		err = fmt.Errorf("creating stdin pipe to clangd failed: %v", err)
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("creating stdin pipe to clangd failed: %v", err)
		return
	}

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		err = fmt.Errorf("starting %s failed: %v", friendlyLspCmdName, err)
		return
	}

	rw := ReadWriter{stdout, stdin}
	stream := rpc.NewStream(rw)
	client = rpc.NewConn(stream)

	client.Go(context.Background(), handler)

	return
}
