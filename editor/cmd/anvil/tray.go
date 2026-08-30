package main

import (
	"fmt"
	"sort"
	"strings"
)

type trayScope int

const (
	trayScopeUnknown trayScope = iota
	trayScopeGlobal
	trayScopeSession
	trayScopeFile
)

var sessionScopedTrays = NewTrays(trayScopeSession, WindowStyle.trayStyle())

// fileScopedTrays is a map of directory paths to the trays contained in that directory
var fileScopedTrays = make(FileScopedTrays)
var globalScopedTrays = NewTrays(trayScopeGlobal, WindowStyle.trayStyle())

type Trays struct {
	floats map[string]*Float
	scope  trayScope
	style  floatStyle
}

func NewTrays(scope trayScope, style floatStyle) Trays {
	return Trays{scope: scope, style: style}
}

// Get gets or creates the tray with the specified name and returns it in `tray`.
// If the tray was created by this call then created is set to true.
func (r *Trays) Get(name string) (tray *Float, created bool) {
	r.init()

	tray, ok := r.floats[name]
	if !ok {
		tray = r.add(name, r.defaultText())
		created = true
	}
	return
}

func (r *Trays) add(name, text string) *Float {
	r.init()

	f := NewFloat(editor.layout.style.trayStyle(), editor.work)
	f.opts.ShowAllLines = true
	f.content.SetTextString(text)
	r.floats[name] = f
	return f
}

func (r *Trays) init() {
	if r.floats == nil {
		r.floats = make(map[string]*Float)
	}
}

func (r *Trays) defaultText() string {
	return `    
    
    `
}

func (r *Trays) Clear() {
	r.floats = nil
}

func (r *Trays) Save() {
	for name, flt := range r.floats {
		saveTrayContents(name, r.scope, flt)
	}
}

func saveTrayContents(trayName string, scope trayScope, f *Float) {
	trayFile, anvilDir, trayDir, err := trayFilePath(trayName, scope, f)
	if err != nil {
		log(LogCatgTrays, "saveTrayContents: can't determine tray file path: %v\n", err)
		return
	}

	sfs, err := GetFs(trayFile.String())
	if err != nil {
		log(LogCatgTrays, "saveTrayContents: can't get fs: %v. Aborting\n", err)
		return
	}

	go func() {
		// Ignore errors here like when the directory already exists
		if anvilDir != nil {
			log(LogCatgTrays, "saveTrayContents: making directory %s\n", anvilDir.String())
			sfs.mkdir(anvilDir.String())
		}
		if trayDir != nil {
			log(LogCatgTrays, "saveTrayContents: making directory %s\n", trayDir.String())
			sfs.mkdir(trayDir.String())
		}

		err := sfs.saveFile(trayFile.String(), f.content.Bytes())
		if err != nil {
			fn := func() {
				d := ""
				if trayDir != nil {
					d = trayDir.String()
				}
				editor.AppendError(d, fmt.Sprintf("error saving tray '%s': %v", trayName, err))
			}
			editor.WorkChan() <- basicWork{fn}
		}
	}()
}

func trayFilePath(trayName string, scope trayScope, f *Float) (filePath, anvilDir, trayDir *GlobalPath, err error) {
	if trayName == "" {
		err = fmt.Errorf("Can't save an unnamed tray")
		return
	}

	if f.evoker == nil {
		err = fmt.Errorf("Float's evoker is nil")
		return
	}

	if scope == trayScopeSession {
		err = fmt.Errorf("Session scope trays are not saved in a file")
		return
	}

	if scope == trayScopeFile {
		var dir *GlobalPath
		dir, err = fileScopedTrayDirectory(f.evoker)
		if err != nil {
			return
		}

		anvilDir = dir.Join(".anvil")
		trayDir = dir.Join(".anvil/trays")
	} else if scope == trayScopeGlobal {
		path := NewGlobalPath(ConfDir, GlobalPathIsDir)
		trayDir = path.Join("trays")
	}

	fname := trayName
	if fname[0] == '§' {
		fname = fname[1:]
	}

	filePath = trayDir.Join(fname)
	return
}

// fileScopedTrayDirectory returns the directory in which to find or create the file-scoped trays directory .anvil/trays for the given loadPath
func fileScopedTrayDirectory(evoker *editable) (dir *GlobalPath, err error) {
	if evoker == nil {
		err = fmt.Errorf("Float's evoker is nil")
		return
	}

	loadPath := evoker.adapter.loadPath()
	if loadPath == nil {
		err = fmt.Errorf("Float's evoker's loadpath is nil")
		return
	}

	if loadPath.DirState() == GlobalPathUnknown {
		err = fmt.Errorf("can't tell if evoker loadpath is a directory or not.")
		return
	}

	dir = loadPath.Dir()
	return
}

func fileScopedTraysForTray(evoker *editable) (*Trays, error) {
	dir, err := fileScopedTrayDirectory(evoker)
	if err != nil {
		return nil, err
	}

	d := dir.String()
	r, ok := fileScopedTrays[d]
	if !ok {
		t := NewTrays(trayScopeFile, WindowStyle.trayStyle())
		r = &t
		fileScopedTrays[d] = r
	}
	return r, nil
}

func loadTrayContents(trayName string, scope trayScope, f *Float) {
	var makeWork func(job Job, ed *editable, data []byte, first bool) Work

	makeWork = func(job Job, ed *editable, data []byte, first bool) Work {
		return &edAppend{job: job, ed: ed, data: data, first: first}
	}

	trayFile, _, _, err := trayFilePath(trayName, scope, f)
	if err != nil {
		log(LogCatgTrays, "loadTrayContents: can't determine tray file path: %v\n", err)
		return
	}

	var ldr FileLoader
	load, err := ldr.LoadAsync(trayFile.String())
	if err != nil {
		log(LogCatgTrays, "loadTrayContents: creating file load failed: %v\n", err)
		return
	}

	wl := &EditableModify{
		DataLoad: *load,
		Jobname:  trayName,
		Editable: &f.content.editable,
		MakeWork: makeWork,
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)
}

func (r *Trays) Names() []string {
	var l []string
	for k := range r.floats {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}

func (r *Trays) Del(name string) (found bool) {
	r.init()

	_, ok := r.floats[name]
	if ok {
		delete(r.floats, name)
		found = true
	}

	return
}

func (r Trays) SetStyle(style Style) {
	for _,f := range r.floats {
		f.SetStyle(style)
	}
}

func parseTrayCmd(cmd string) (name string, scope trayScope) {
	parts := strings.Split(cmd, ":")
	if len(parts) == 1 {
		name = parts[0]
		return
	}

	if strings.ContainsRune(parts[0], 'g') {
		scope = trayScopeGlobal
	} else if strings.ContainsRune(parts[0], 's') {
		scope = trayScopeSession
	} else if strings.ContainsRune(parts[0], 'f') {
		scope = trayScopeFile
	}

	name = parts[1]
	return
}

type FileScopedTrays map[string]*Trays

func (f FileScopedTrays) Save() {
	for _, r := range f {
		r.Save()
	}
}

func (f FileScopedTrays) SetStyle(style Style) {
	for _, r := range f {
		r.SetStyle(style)
	}
}
