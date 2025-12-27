package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"github.com/flopp/go-findfont"
	"github.com/jeffwilliams/anvil/editor/internal/ansi"
	"github.com/jeffwilliams/anvil/editor/internal/draw"
	"github.com/jeffwilliams/anvil/editor/internal/typeset"
	toml "github.com/pelletier/go-toml"
)

var ConfDir string

func init() {
	var d string
	if runtime.GOOS == "windows" {
		d = os.Getenv("USERPROFILE")
	} else {
		d = os.Getenv("HOME")
	}
	ConfDir = filepath.Join(d, ".anvil")
}

func SshKeyDir() string {
	return filepath.Join(ConfDir, "sshkeys")
}

func LoadSshKeys() {
	d := SshKeyDir()
	entries, err := os.ReadDir(d)
	if err != nil {
		log(LogCatgConf, "Error reading sshkeys directory: %v\n", err)
		return
	}

	for _, e := range entries {
		log(LogCatgConf, "Loading ssh key %s\n", e.Name())
		path := filepath.Join(d, e.Name())
		sshClientCache.AddKeyFromFile(e.Name(), path)
	}
}

func StyleConfigFile() string {
	return filepath.Join(ConfDir, "style.js")
}

func loadFontFromFile(filename string) (f text.FontFace, err error) {
	log(LogCatgConf, "Loading font %s from file\n", filename)
	path, err := findfont.Find(filename)
	if err != nil {
		return
	}

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	var face opentype.Face
	face, err = typeset.ParseTTF(file)
	if err != nil {
		return
	}

	return fontFaceFromOpentype(face, filepath.Base(filename)), nil
}

func fontFaceFromOpentype(face opentype.Face, typefaceName string) text.FontFace {
	ff := text.FontFace{
		Font: font.Font{
			Typeface: font.Typeface(typefaceName),
		},
		Face: face,
	}

	return ff

}

func LoadStyleFromConfigFile(defaults *Style) (s Style, err error) {
	return LoadStyleFromFile(StyleConfigFile(), defaults)
}

func LoadStyleFromFile(path string, defaults *Style) (s Style, err error) {
	s, err = ReadStyle(path, defaults)
	if err != nil {
		return
	}

	for i, f := range s.Fonts {
		s.Fonts[i].FontFace = VariableFont
		if f.FontName != "" {

			if f.FontName == "defaultMonoFont" {
				s.Fonts[i].FontFace = MonoFont
				continue
			}

			if f.FontName == "defaultVariableFont" {
				s.Fonts[i].FontFace = VariableFont
				continue
			}

			var fnt text.FontFace
			fnt, err = loadFontFromFile(f.FontName)
			if err != nil {
				return
			}
			s.Fonts[i].FontFace = fnt
		}
	}

	ops, err := draw.Parse(s.CursorProg)
	if err != nil {
		err = fmt.Errorf("parsing CursorProg property from style failed: %v", err)
		return
	}
	s.compiledCursorProg = ops

	return

}

func LoadCurrentStyleFromFile(path string, defaults *Style) (err error) {
	s, err := LoadStyleFromFile(path, defaults)
	if err != nil {
		return err
	}
	WindowStyle = s
	ansi.InitColors(WindowStyle.Ansi.AsColors())
	editor.SetStyle(WindowStyle)

	return
}

func SaveCurrentStyleToFile(path string) (err error) {
	err = WriteStyle(path, WindowStyle)
	return
}

func PlumbingConfigFile() string {
	return filepath.Join(ConfDir, "plumbing")
}

func LoadPlumbingRulesFromFile(path string) (rules []PlumbingRule, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}

	rules, err = ParsePlumbingRules(f)

	defer f.Close()
	return
}

func SettingsConfigFile() string {
	return filepath.Join(ConfDir, "settings.toml")
}

type Settings struct {
	Ssh         SshSettings
	Typesetting TypesettingSettings
	Layout      LayoutSettings
	General     GeneralSettings
	Env         map[string]string
	Alias       map[string]string
}

type SshSettings struct {
	Shell             string
	CloseStdin        bool `toml:"close-stdin"`
	CacheSize         int
	ConnectionTimeout int `toml:"conn-timeout"`
}

type TypesettingSettings struct {
	ReplaceCRWithTofu bool `toml:"replace-cr-with-tofu"`
}

func LoadSettingsFromConfigFile(settings *Settings) (err error) {
	var f *os.File
	f, err = os.Open(SettingsConfigFile())
	if err != nil {
		return
	}
	defer f.Close()

	dec := toml.NewDecoder(f)

	err = dec.Decode(settings)
	return

}

type LayoutSettings struct {
	EditorTag         string `toml:"editor-tag"`
	ColumnTag         string `toml:"column-tag"`
	WindowTagUserArea string `toml:"window-tag-user-area"`
}

type GeneralSettings struct {
	ExecuteOnStartup []string `toml:"exec"`
}

func GenerateSampleSettings() string {
	return `# Sample anvil settings file
[general]
# exec is a list of commands to run when Anvil is started.
#exec=[
# "aad",
# "ado"
#]

[layout]
# The default part of the editor tag that does not include running commands
#editor-tag="Newcol Kill Putall Dump Load Exit Help ◊ "

# The default column tag
#column-tag="New Cut Paste Snarf Zerox Delcol "

# The default part of the window tag that the user can edit
#window-tag-user-area=" Do Look "

[typesetting]
# When rendering text show carriage-returns as the "tofu" character (a box)
# The default is false
#replace-cr-with-tofu=false

# The env table lists environment variables to be exported when running
# commands.
#[env]
#VAR="val"

[ssh]
# shell specifies the shell to use when commands are executed on a remote system.
# The default is "sh"
#shell="sh"

# close-stdin controls if stdin is closed for remote programs when they are executed
# using middle-click. Some programs require this to operate properly such as ripgrep,
# while some require stdin to be open even if it is not read, like man or git.
# The default is false
#close-stdin=false

# cachesize is the max number of ssh sessions kept open at once. Each user, host, port, proxy
# combination requires a different connection
#cachesize=5

# conntimeout is the TCP connection timeout for the SSH session in seconds
#conn-timeout=5

# The alias table lists command aliases. The key is the name of the alias and the
# value are the commands to run separated by semicolon (;).
[alias]
`
}

// DoMatchConfigFileParser knows how to parse a file that has do and match lines, like
// the plumbing file.
func parseDoMatchConfigFile(f io.Reader, onMatch func(re *regexp.Regexp), onDo func(do string)) (err error) {

	s := bufio.NewScanner(f)

	const (
		stateExpectMatch = iota
		stateExpectDo
	)

	state := stateExpectMatch

	for s.Scan() {
		line := s.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		toks := strings.SplitN(line, " ", 2)
		if len(toks) < 2 {
			err = fmt.Errorf("Invalid line: expected a word, a space, then a string. Line is '%s'", line)
			return
		}

		switch state {
		case stateExpectMatch:
			if toks[0] != "match" {
				err = fmt.Errorf("Expected line beginning with 'match' but got '%s'", line)
				return
			}
			re, err2 := regexp.Compile(toks[1])
			if err2 != nil {
				err = fmt.Errorf("Parsing regexp for line '%s' failed: %v", line, err)
				return
			}
			onMatch(re)
			state = stateExpectDo
		case stateExpectDo:
			if toks[0] != "do" {
				err = fmt.Errorf("Expected line beginning with 'do' but got '%s'", line)
				return
			}
			onDo(toks[1])
			state = stateExpectMatch
		}
	}
	return
}

func KeymapConfigFile() string {
	return filepath.Join(ConfDir, "keymaps")
}
