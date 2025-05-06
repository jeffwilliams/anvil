package main

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// GlobalPath represents a path that might be remote (on another host) or local.
type GlobalPath struct {
	user, host, path, port          string
	proxyUser, proxyHost, proxyPort string
	dirState                        GlobalPathDirState
}

type GlobalPathDirState int

const (
	GlobalPathUnknown GlobalPathDirState = iota
	GlobalPathIsDir
	GlobalPathIsFile
)

func NewGlobalPath(path string, dsr GlobalPathDirState) (p *GlobalPath) {
	p = &GlobalPath{}
	p.splitFilename(path)
	p.dirState = dsr
	return
}

func NewLocalPath(path string, dsr GlobalPathDirState) *GlobalPath {
	return &GlobalPath{path: path, dirState: dsr}
}

func (g *GlobalPath) splitFilename(path string) {
	colonIndex := strings.Index(path, ":")
	if colonIndex < 0 || isWindowsPath(path) {
		g.path = path
		return
	}

	// Grammar:
	// GlobalPath -> Dest? Path | Dest Proxy Path
	// Dest -> (User '@')? Host ':'
	// Proxy -> '%' (User '@')? Host ':'

	parseHop := func(s string) (user, host, port, rest string) {
		i := 0

		consumePrefix := func() (r string) {
			r = s[0:i]
			s = s[i+1:]
			i = 0
			return
		}

		sawColon := false

		for i < len(s) {
			r := s[i]
			if r == '@' && !sawColon {
				user = consumePrefix()
				continue
			} else if r == ':' {
				sawColon = true
				if host == "" {
					host = consumePrefix()
					continue
				} else if port == "" {
					port = consumePrefix()
					continue
				}
			}
			i++
		}
		rest = s
		return
	}

	pctIndex := strings.Index(path, "%")
	if pctIndex >= 0 {
		// Contains a proxy.
		if pctIndex == 0 {
			// Proxy is empty; just treat it as the host
			g.user, g.host, g.port, g.path = parseHop(path[1:])
			return
		}
		g.user, g.host, g.port, _ = parseHop(path[:pctIndex] + ":")
		g.proxyUser, g.proxyHost, g.proxyPort, g.path = parseHop(path[pctIndex+1:])
		return
	}

	g.user, g.host, g.port, g.path = parseHop(path)
	return
}

func (g *GlobalPath) SetDirState(s GlobalPathDirState) {
	g.dirState = s
}
func (g *GlobalPath) DirState() GlobalPathDirState {
	return g.dirState
}

func (g GlobalPath) Host() string {
	return g.host
}

func (g *GlobalPath) SetHost(x string) {
	g.host = x
}

func (g GlobalPath) User() string {
	return g.user
}

func (g *GlobalPath) SetUser(x string) {
	g.user = x
}

func (g GlobalPath) Path() string {
	return g.path
}

func (g *GlobalPath) SetPath(path string) {
	g.path = path
}

func (g GlobalPath) Port() string {
	return g.port
}

func (g *GlobalPath) SetPort(x string) {
	g.port = x
}

func (g GlobalPath) ProxyUser() string {
	return g.proxyUser
}

func (g GlobalPath) ProxyHost() string {
	return g.proxyHost
}

func (g GlobalPath) ProxyPort() string {
	return g.proxyPort
}

func (g GlobalPath) IsRemote() bool {
	return g.host != ""
}

func (g GlobalPath) HasProxy() bool {
	return g.proxyHost != ""
}

func (g GlobalPath) String() string {
	proxyStr := ""
	if g.proxyHost != "" {
		var buf bytes.Buffer
		if g.proxyUser != "" {
			fmt.Fprintf(&buf, "%s@", g.proxyUser)
		}

		fmt.Fprintf(&buf, "%s:", g.proxyHost)

		if g.proxyPort != "" {
			fmt.Fprintf(&buf, "%s:", g.proxyPort)
		}

		proxyStr = buf.String()
	}

	if g.host != "" {
		var buf bytes.Buffer
		if g.user != "" {
			fmt.Fprintf(&buf, "%s@", g.user)
		}

		fmt.Fprintf(&buf, "%s", g.host)

		if g.port != "" {
			fmt.Fprintf(&buf, ":%s", g.port)
		}

		if proxyStr != "" {
			fmt.Fprintf(&buf, "%%%s", proxyStr)
		} else {
			buf.WriteRune(':')
		}

		fmt.Fprintf(&buf, "%s", g.path)
		return buf.String()
	} else {
		return g.path
	}
}

// MakeAbsoluteRelativeTo returns the path where p is the prefix to g, if possible.
// Note that g must be a relative path.
// TODO: Rename this to JoinTo
func (g GlobalPath) MakeAbsoluteRelativeTo(p *GlobalPath) *GlobalPath {
	if g.IsRemote() && ((g.user != p.user) || (g.host != p.host) || (g.port != p.port)) {
		// This is not supported
		return &g
	}

	// If this is a remote file but we are running on windows, we _dont_
	// want to convert / to \ in the path.
	join := filepath.Join
	if p.IsRemote() {
		join = path.Join
	}
	dirFunc := filepath.Dir
	if p.IsRemote() {
		dirFunc = path.Dir
	}

	var path string
	if p.dirState == GlobalPathIsDir {
		path = join(p.path, g.path)
	} else {
		path = join(dirFunc(p.path), g.path)
	}

	return &GlobalPath{
		user:      p.user,
		host:      p.host,
		path:      path,
		port:      p.port,
		dirState:  g.dirState,
		proxyUser: p.proxyUser,
		proxyHost: p.proxyHost,
		proxyPort: p.proxyPort,
	}
}

// Dir returns the directory of the GlobalPath. If `g` is a directory it is returned,
// otherwise it's parent directory is returned
func (g GlobalPath) Dir() *GlobalPath {
	// If this is a remote file but we are running on windows, we _dont_
	// want to convert / to \ in the path.
	dir := filepath.Dir
	if g.IsRemote() {
		dir = path.Dir
	}

	path := g.path
	if g.dirState != GlobalPathIsDir {
		path = dir(g.path)
	}

	return &GlobalPath{
		user:      g.user,
		host:      g.host,
		port:      g.port,
		path:      path,
		dirState:  GlobalPathIsDir,
		proxyUser: g.proxyUser,
		proxyHost: g.proxyHost,
		proxyPort: g.proxyPort,
	}
}

// Base returns the last element of the GlobalPath.
func (g GlobalPath) Base() string {
	// If this is a remote file but we are running on windows, we _dont_
	// want to convert / to \ in the path.
	base := filepath.Base
	if g.IsRemote() {
		base = path.Base
	}

	return base(g.path)
}

func (g GlobalPath) IsAbsolute() bool {
	// If we are running on windows we still want to consider a file
	// starting with / as absolute, even though it doesn't have a drive,
	// since this might be a remote file.
	isAbsFunc := filepath.IsAbs
	if g.IsRemote() {
		isAbsFunc = path.IsAbs
	}

	return isAbsFunc(g.path)
}

// GlobalizeRelativeTo makes g a global path, using the remote properties from p.
// Specifically, if g is a local path and p is global then g is made global by
// setting its user, host, port and proxy settings to those properties from p.
// If g is already global when this function is called, it is not changed.
func (g GlobalPath) GlobalizeRelativeTo(p *GlobalPath) *GlobalPath {
	if g.IsRemote() || !p.IsRemote() {
		return &g
	}

	return &GlobalPath{
		user:      p.user,
		host:      p.host,
		path:      g.path,
		port:      p.port,
		dirState:  g.dirState,
		proxyUser: p.proxyUser,
		proxyHost: p.proxyHost,
		proxyPort: p.proxyPort,
	}
}

func (g GlobalPath) Clone() *GlobalPath {
	return &GlobalPath{
		user:      g.user,
		host:      g.host,
		path:      g.path,
		port:      g.port,
		dirState:  g.dirState,
		proxyUser: g.proxyUser,
		proxyHost: g.proxyHost,
		proxyPort: g.proxyPort,
	}
}

func (g *GlobalPath) EnsureDirEndsInSlash() {
	if g.dirState != GlobalPathIsDir {
		return
	}

	sep := string(filepath.Separator)
	if g.IsRemote() {
		sep = "/"
	}

	if !strings.HasSuffix(g.path, sep) {
		g.path = g.path + sep
	}
}

// MakeRelativeUsingPrefix if p is a prefix of g, then this function returns the
// suffix of g with p removed. If p is not a prefix of g, nil is returned
func (g *GlobalPath) MakeRelativeUsingPrefix(p *GlobalPath) *GlobalPath {
	if p.dirState != GlobalPathIsDir {
		return nil
	}

	gs := g.String()
	if gs == p.String() {
		return &GlobalPath{
			user:      g.user,
			host:      g.host,
			path:      ".",
			port:      g.port,
			dirState:  g.dirState,
			proxyUser: g.proxyUser,
			proxyHost: g.proxyHost,
			proxyPort: g.proxyPort,
		}
	}

	pc := p.Clone()
	pc.EnsureDirEndsInSlash()

	ps := pc.String()

	if !strings.HasPrefix(gs, ps) {
		return nil
	}

	path := gs[len(ps):]

	return &GlobalPath{
		path:     path,
		dirState: g.dirState,
	}

}
