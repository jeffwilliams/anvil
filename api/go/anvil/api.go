// Package anvil implements an API for interacting with Anvil.
//
// The general usage is to create an Anvil struct using the New or NewFromEnv function, then call various
// Get, GetInto or Put methods on Anvil.

package anvil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

func checkHttpError(rsp *http.Response, msg string) error {
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(rsp.Body)
		return fmt.Errorf("%s: response contained a non-success status code (%d) %s\n", msg, rsp.StatusCode, string(body))
	}
	return nil
}

func prefixError(err error, msg string) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %s", msg, err.Error())
}

type URLs struct {
	Host  string
	Proto string
	Port  string
}

func NewURLs(port string) URLs {
	return URLs{
		Proto: "http",
		Host:  "localhost",
		Port:  port,
	}
}

func (u URLs) Build(path string) string {
	return fmt.Sprintf("%s://%s:%s%s", u.Proto, u.Host, u.Port, path)
}

type Anvil struct {
	sessId string
	urls   URLs
	client http.Client
}

func New(sessId, port string) Anvil {
	return Anvil{
		sessId: sessId,
		urls:   NewURLs(port),
	}
}

func NewFromEnv() (anvil Anvil, err error) {
	sessId := os.Getenv("ANVIL_API_SESS")
	port := os.Getenv("ANVIL_API_PORT")

	if sessId == "" {
		err = fmt.Errorf("environment variable ANVIL_API_SESS is not set")
		return
	}
	if port == "" {
		err = fmt.Errorf("environment variable ANVIL_API_PORT is not set")
		return
	}

	anvil = Anvil{
		sessId: sessId,
		urls:   NewURLs(port),
	}
	return
}

// Get is a low-level API that performs an HTTP GET request to
// Anvil and returns the response.
func (a Anvil) Get(path string) (rsp *http.Response, err error) {
	req, url, err := a.buildReq(http.MethodGet, path, nil)
	if err != nil {
		return
	}

	rsp, err = a.client.Do(req)
	err = prefixError(err, fmt.Sprintf("GET to %s failed", url))
	if err != nil {
		return
	}
	err = checkHttpError(rsp, fmt.Sprintf("GET to %s failed", url))
	return
}

// GetInto is a low-level API that performs an HTTP GET request to
// Anvil and decodes the response into resp. It is decoded using
// the encoding/json package.
func (a Anvil) GetInto(path string, resp interface{}) (err error) {
	rsp, err := a.Get(path)
	raw, err := ioutil.ReadAll(rsp.Body)
	err = prefixError(err, "Error reading body info")
	if err != nil {
		return
	}
	err = json.Unmarshal(raw, resp)
	err = prefixError(err, fmt.Sprintf("Error decoding JSON GET response body, body is '%s'", raw))
	return
}

// Post is a low-level API that performs an HTTP POST request to
// Anvil with the body in `body` and returns the response. Body should
// usually be a reader that reads a JSON encoded value.
func (a Anvil) Post(path string, body io.Reader) (rsp *http.Response, err error) {
	req, url, err := a.buildReq(http.MethodPost, path, body)
	if err != nil {
		return
	}

	rsp, err = a.client.Do(req)
	err = prefixError(err, fmt.Sprintf("POST to %s failed", url))
	if err != nil {
		return
	}
	err = checkHttpError(rsp, fmt.Sprintf("POST to %s failed", url))
	return

}

// Put is a low-level API that performs an HTTP PUT request to
// Anvil with the body in `body` and returns the response. Body should
// usually be a reader that reads a JSON encoded value.
func (a Anvil) Put(path string, body io.Reader) (rsp *http.Response, err error) {
	req, url, err := a.buildReq(http.MethodPut, path, body)
	if err != nil {
		return
	}

	rsp, err = a.client.Do(req)
	err = prefixError(err, fmt.Sprintf("PUT to %s failed", url))
	if err != nil {
		return
	}
	err = checkHttpError(rsp, fmt.Sprintf("PUT to %s failed", url))
	return
}

func (a Anvil) buildReq(method, path string, body io.Reader) (req *http.Request, url string, err error) {
	url = a.urls.Build(path)
	req, err = http.NewRequest(method, url, body)
	err = prefixError(err, fmt.Sprintf("Error building %s request for %s", method, url))
	if err != nil {
		return
	}

	a.setHeaderFields(&req.Header)
	return
}

func (a Anvil) setHeaderFields(hdr *http.Header) {
	hdr.Add("Anvil-Sess", a.sessId)
	hdr.Add("Content-Type", "application/json")
	hdr.Add("Accept", "application/json")
}

// Websock creates a websocket connection with Anvil to receive notifications. The handlers
// in `handlers` are called when notifications arrive from Anvil.
func (a Anvil) Websock(handlers WebsockHandlers) (ws Websock, err error) {
	dialer := websocket.Dialer{}
	hdr := make(http.Header)
	a.setHeaderFields(&hdr)

	urls := a.urls
	urls.Proto = "ws"
	url := urls.Build("/ws")

	var conn *websocket.Conn
	conn, _, err = dialer.Dial(url, hdr)

	err = prefixError(err, fmt.Sprintf("GET to %s failed", url))
	if err != nil {
		return
	}

	ws = Websock{
		conn:     conn,
		handlers: handlers,
	}
	return
}

// Execute is a high-level API to post to /execute in Anvil, which executes
// a command. It is run as if it was run from the editor tag.
func (a Anvil) Execute(command string, args []string) (err error) {
	val := map[string]interface{}{
		"cmd":   command,
		"args":  args,
		"winid": -1,
	}
	b, err := json.Marshal(val)
	if err != nil {
		err = fmt.Errorf("marshalling command to JSON failed: %v", err)
		return
	}

	cmd := bytes.NewReader(b)
	_, err = a.Post("/execute", cmd)

	return
}

// Execute is a high-level API to post to /execute in Anvil, which executes
// a command. It is run as if executed in the specified window.
func (a Anvil) ExecuteInWin(win Window, command string, args []string) (err error) {
	val := map[string]interface{}{
		"cmd":   command,
		"args":  args,
		"winid": win.Id,
	}
	b, err := json.Marshal(val)
	if err != nil {
		err = fmt.Errorf("marshalling command to JSON failed: %v", err)
		return
	}

	cmd := bytes.NewReader(b)
	_, err = a.Post("/execute", cmd)

	return
}

// Windows is a high-level API to get from /wins in Anvil, which returns
// the windows
func (a Anvil) Windows() (wins []Window, err error) {
	err = a.GetInto("/wins", &wins)
	if err != nil {
		return
	}

	return
}

var noBody io.Reader

// NewWindow is a high-level API to post to /wins in Anvil, which creates
// a new window and returns it
func (a Anvil) NewWindow() (win Window, err error) {

	rsp, err := a.Post("/wins", noBody)
	if err != nil {
		err = fmt.Errorf("creating new window failed", err)
		return
	}

	raw, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		err = fmt.Errorf("reading response from creating window failed", err)
		return
	}

	err = json.Unmarshal(raw, &win)
	if err != nil {
		err = fmt.Errorf("decoding JSON response after creating window failed", err)
		return
	}
	return
}

// Window is a high-level API to get from /wins/%d/info/ in Anvil, which returns
// the information about the window with the given id
func (a Anvil) Window(id int) (win Window, err error) {
	err = a.GetInto(fmt.Sprintf("/wins/%d/info", id), &win)
	return
}

// WindowTag is a high-level API to get from /wins/%d/tag in Anvil, which
// returns the window tag
func (a Anvil) WindowTag(win Window) (tag string, err error) {
	rsp, err := a.Get(fmt.Sprintf("/wins/%d/tag", win.Id))
	if err == nil {
		var b []byte
		b, err = ioutil.ReadAll(rsp.Body)
		tag = string(b)
	}
	return
}

// SetWindowTag is a high-level API to post to /wins/%d/tag, which sets
// the window tag
func (a Anvil) SetWindowTag(win Window, tag string) (err error) {
	var buf bytes.Buffer
	buf.WriteString(tag)
	_, err = a.Put(fmt.Sprintf("/wins/%d/tag", win.Id), &buf)
	return
}

func (a Anvil) SetWindowBody(win Window, body io.Reader) (err error) {
	_, err = a.Put(fmt.Sprintf("/wins/%d/body", win.Id), body)
	return
}

func (a Anvil) SetWindowBodyString(win Window, body string) (err error) {
	var buf bytes.Buffer
	buf.WriteString(body)
	_, err = a.Put(fmt.Sprintf("/wins/%d/body", win.Id), &buf)
	return
}

func (a Anvil) AppendToWindowBody(win Window, body io.Reader) (err error) {
	_, err = a.Post(fmt.Sprintf("/wins/%d/body", win.Id), body)
	return
}

func (a Anvil) AppendToWindowBodyString(win Window, body string) (err error) {
	var buf bytes.Buffer
	buf.WriteString(body)
	_, err = a.Post(fmt.Sprintf("/wins/%d/body", win.Id), &buf)
	return
}

func (a Anvil) InsertToWindowBody(win Window, body io.Reader) (err error) {
	_, err = a.Put(fmt.Sprintf("/wins/%d/body/insert", win.Id), body)
	return
}

func (a Anvil) InsertToWindowBodyString(win Window, body string) (err error) {
	var buf bytes.Buffer
	buf.WriteString(body)
	_, err = a.Put(fmt.Sprintf("/wins/%d/body/insert", win.Id), &buf)
	return
}

func (a Anvil) WindowBody(win Window) (body io.Reader, err error) {
	rsp, err := a.Get(fmt.Sprintf("/wins/%d/body", win.Id))
	body = rsp.Body
	return
}

func (a Anvil) WindowBodyInfo(win Window) (body WindowBody, err error) {
	err = a.GetInto(fmt.Sprintf("/wins/%d/body/info", win.Id), &body)
	return
}

func (a Anvil) WindowBodySelections(win Window) (sels []Selection, err error) {
	err = a.GetInto(fmt.Sprintf("/wins/%d/selections", win.Id), &sels)
	return
}

func (a Anvil) WindowBodyCursors(win Window) (cursors []int, err error) {
	err = a.GetInto(fmt.Sprintf("/wins/%d/body/cursors", win.Id), &cursors)
	return
}

func (a Anvil) RegisterCommands(names ...string) error {
	var buf bytes.Buffer
	l := strings.Join(names, `","`)
	buf.WriteString(fmt.Sprintf(`["%s"]`, l))
	_, err := a.Post("/cmds", &buf)
	return err
}

func (a Anvil) Info() (info AnvilInfo, err error) {
	err = a.GetInto("/info", &info)
	return
}

// WindowBodyWriter is a high-level API that returns an io.Writer that when written to
// appends to the body of the specified window.
func (a Anvil) WindowBodyWriter(win Window) io.Writer {
	return newBodyWriter(a, win.Id)
}
