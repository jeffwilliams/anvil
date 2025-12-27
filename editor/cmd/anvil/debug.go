package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"
)

const (
	LogCatgApp        = "Application"
	LogCatgUI         = "UI"
	LogCatgEd         = "Editable"
	LogCatgSyntax     = "Syntax"
	LogCatgAPI        = "API"
	LogCatgFS         = "Filesystem"
	LogCatgCompletion = "Completion"
	LogCatgPlumb      = "Plumbing"
	LogCatgWin        = "Window"
	LogCatgCmd        = "Commands"
	LogCatgCol        = "Column"
	LogCatgConf       = "Config"
	LogCatgEditor     = "Editor"
	LogCatgLayer      = "Layer"
	LogCatgPack       = "Packing"
	LogCatgSsh        = "SSH"
	LogCatgExpr       = "Expressions"
	LogCatgFuzzy      = "FuzzySearch"
)

var debugLogCategories = []string{
	LogCatgApp,
	LogCatgUI,
	LogCatgEd,
	LogCatgSyntax,
	LogCatgAPI,
	LogCatgFS,
	LogCatgCompletion,
	LogCatgPlumb,
	LogCatgWin,
	LogCatgCmd,
	LogCatgCol,
	LogCatgConf,
	LogCatgEditor,
	LogCatgPack,
	LogCatgSsh,
	LogCatgExpr,
	LogCatgFuzzy,
}

var killPprofDebugServer = make(chan struct{})

func startPprofDebugServer() {
	go func() {
		server := &http.Server{Addr: "localhost:6060"}

		go func() {
			<-killPprofDebugServer
			server.Close()
		}()

		err := server.ListenAndServe()

		if err != nil && err.Error() != "http: Server closed" {
			w := basicWork{func() {
				editor.AppendError("", fmt.Sprintf("Error starting pprof debug server: %v %T", err, err))
			}}
			editor.WorkChan() <- w
			return
		}

	}()
}

func stopPprofDebugServer() {
	select {
	case killPprofDebugServer <- struct{}{}:
	default:
	}
}

func serveFlamegraph(svg []byte) (url string, server *http.Server, err error) {
	var l net.Listener
	l, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}

	tl, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		err = fmt.Errorf("listener is not a *net.TCPAddr. Can't determine port.")
		return
	}

	port := tl.Port

	fs := TrivialFs{
		"/fg": NewByteSliceFile("fg", svg),
	}

	server = new(http.Server)
	server.Handler = http.FileServer(fs)
	go server.Serve(l)
	//go http.Serve(l)

	url = fmt.Sprintf("http://127.0.0.1:%d/fg", port)

	return
}

type TrivialFs map[string]*ByteSliceFile

func (fs TrivialFs) Open(name string) (http.File, error) {
	f, ok := fs[name]
	if !ok {
		return nil, fmt.Errorf("no such file")
	}

	return f, nil
}

type ByteSliceFile struct {
	reader  *bytes.Reader
	name    string
	modTime time.Time
}

func NewByteSliceFile(name string, data []byte) *ByteSliceFile {
	return &ByteSliceFile{
		reader:  bytes.NewReader(data),
		name:    name,
		modTime: time.Now(),
	}
}

func (f *ByteSliceFile) Read(p []byte) (n int, err error) {
	return f.reader.Read(p)
}

func (f *ByteSliceFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}

func (f *ByteSliceFile) Close() error {
	return nil
}

func (f *ByteSliceFile) Readdir(count int) ([]fs.FileInfo, error) {
	return []fs.FileInfo{}, fmt.Errorf("Not a directory")
}

func (f *ByteSliceFile) Stat() (fs.FileInfo, error) {
	return f, nil
}

func (f ByteSliceFile) Name() string {
	return f.name
}

func (f ByteSliceFile) Size() int64 {
	return int64(f.reader.Len())
}

func (f ByteSliceFile) Mode() fs.FileMode {
	return 0755
}

func (f ByteSliceFile) ModTime() time.Time {
	return f.modTime
}

func (f ByteSliceFile) IsDir() bool {
	return false
}

func (f ByteSliceFile) Sys() interface{} {
	return nil
}
