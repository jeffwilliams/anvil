package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

func CommandCompletionsAsync(word, dir string, executor *CommandExecutor, callback CompletionsCallback) error {
	sfs, err := GetFs(dir)
	if err != nil {
		return err
	}

	j := CommandCompletionJob{
		sfs:      sfs,
		dir:      dir,
		word:     word,
		work:     editor.WorkChan(),
		callback: callback,
		executor: executor,
	}

	editor.AddJob(&j)
	go j.run()
	return nil
}

type CommandCompletionJob struct {
	sfs      simpleFs
	val      chan string
	dir      string
	errs     chan error
	kill     chan struct{}
	word     string
	work     chan Work
	callback CompletionsCallback
	executor *CommandExecutor
}

func (j *CommandCompletionJob) run() {
	pathDirs, err := j.getPath()
	if err != nil {
		j.work <- &appendError{job: j, dir: j.dir, err: err}
		return
	}

	globalPath := NewGlobalPath(j.dir, GlobalPathUnknown)

	cmds := j.filesInDirs(pathDirs, globalPath)
	cmds = j.appendBuiltinCmdsAndAliases(cmds)
	cmds = slice.StringsHavingPrefix(cmds, j.word)
	cmds = removeNonExecutables(cmds)

	j.work <- &applyCompletionsToEditable{job: j,
		completions: cmds,
		word:        j.word,
		callback:    j.callback,
	}
}

func (j *CommandCompletionJob) appendBuiltinCmdsAndAliases(cmds []string) []string {
	for _, v := range j.executor.Commands() {
		cmds = append(cmds, v.name)
	}

	for k, _ := range settings.Alias {
		cmds = append(cmds, k)
	}

	return cmds
}

func (j *CommandCompletionJob) filesInDirsSerial(dirs []string, globalPath *GlobalPath) []string {
	uniqueCmds := make(stringSet)
	for _, dir := range dirs {

		dirGlobalPath := NewGlobalPath(dir, GlobalPathUnknown)
		dirGlobalPath = dirGlobalPath.GlobalizeRelativeTo(globalPath)

		names, err := j.getDirContents(dirGlobalPath.String())

		if err != nil {
			j.work <- &appendError{job: j, dir: j.dir, err: err}
			//return
			continue
		}

		uniqueCmds.Add(names)
	}

	return uniqueCmds.Slice()
}

// filesInDirs returns a list of all unique filenames found directly under a list of directories.
func (j *CommandCompletionJob) filesInDirs(dirs []string, globalPath *GlobalPath) []string {
	uniqueCmds := make(stringSet)

	names, err := j.getDirsContents(globalPath.String(), dirs)

	if err != nil {
		j.work <- &appendError{job: j, dir: j.dir, err: err}
	}

	uniqueCmds.Add(names)

	return uniqueCmds.Slice()
}

func (j *CommandCompletionJob) filesInDirsParallel(dirs []string, globalPath *GlobalPath) []string {

	in := make(chan string)
	out := make(chan []string)
	var wg sync.WaitGroup

	worker := func(n int) {
		for dir := range in {
			fmt.Printf("filesInDirsParallel: worker %d: starting dir\n", n)
			dirGlobalPath := NewGlobalPath(dir, GlobalPathUnknown)
			dirGlobalPath = dirGlobalPath.GlobalizeRelativeTo(globalPath)

			names, err := j.getDirContents(dirGlobalPath.String())

			if err != nil {
				j.work <- &appendError{job: j, dir: j.dir, err: err}
				continue
			}

			fmt.Printf("filesInDirsParallel: worker %d: done dir\n", n)
			out <- names
		}
		wg.Done()
	}

	feeder := func() {
		for _, dir := range dirs {
			in <- dir
		}
		close(in)
	}

	closer := func() {
		wg.Wait()
		close(out)
	}

	workerCount := 20
	wg.Add(workerCount)
	go closer()
	for i := 0; i < workerCount; i++ {
		go worker(i)
	}
	go feeder()

	uniqueCmds := make(stringSet)

	for names := range out {
		uniqueCmds.Add(names)
	}

	return uniqueCmds.Slice()
}

func (j CommandCompletionJob) getPath() (pathDirs []string, err error) {
	val := make(chan string)
	errs := make(chan error)
	kill := make(chan struct{})
	err = j.sfs.getEnvAsync("PATH", j.dir, val, errs, kill)
	if err != nil {
		return nil, err
	}

	errsClosed := false
	valClosed := false

	done := func() bool {
		return errsClosed && valClosed
	}

	var path string

FOR:
	for {
		select {
		case x, ok := <-val:
			if !ok {
				valClosed = true
				if done() {
					break FOR
				}
				break
			}
			path = x

		case x, ok := <-errs:
			if !ok {
				log(LogCatgCompletion, "CommandCompletionJob: errors closed\n")
				errsClosed = true
				j.errs = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "CommandCompletionJob: got error %v\n", x)
			err = x
		}
	}

	if err != nil {
		return
	}

	pathDirs = splitPath(path)
	return
}

func (j CommandCompletionJob) getDirContents(dir string) (fnames []string, err error) {

	load := NewDataLoad()
	err = j.sfs.contentsAsync(dir, load.Filenames, load.Contents, load.Errs, load.Kill)

	if err != nil {
		//j.work <- &appendError{job: j, dir: j.dir, err: err}
		return nil, err
	}

	errsClosed := false
	filenamesClosed := false
	contentsClosed := false

	done := func() bool {
		return (errsClosed && filenamesClosed) || (errsClosed && contentsClosed)
	}

FOR:
	for {
		log(LogCatgCompletion, "CommandCompletionJob: select\n")
		select {
		case _, ok := <-load.Contents:
			if !ok {
				contentsClosed = true
				load.Contents = nil
				if done() {
					break FOR
				}
				break
			}

		case x, ok := <-load.Filenames:
			if !ok {
				log(LogCatgCompletion, "CommandCompletionJob: filenames closed\n")
				filenamesClosed = true
				load.Filenames = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "CommandCompletionJob: got more filenames\n")
			fnames = append(fnames, x...)

		case x, ok := <-load.Errs:
			if !ok {
				log(LogCatgCompletion, "CommandCompletionJob: errors closed\n")
				errsClosed = true
				load.Errs = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "CommandCompletionJob: got error %v\n", x)
			err = x
		}
	}

	return
}

func (j CommandCompletionJob) getDirsContents(rundir string, dirs []string) (fnames []string, err error) {
	load := NewDataLoad()

	err = j.sfs.filenamesInDirsAsync(rundir, dirs, load.Filenames, load.Errs, load.Kill)

	if err != nil {
		return nil, err
	}

	errsClosed := false
	filenamesClosed := false

	done := func() bool {
		return errsClosed && filenamesClosed
	}

FOR:
	for {
		log(LogCatgCompletion, "CommandCompletionJob: select\n")
		select {
		case x, ok := <-load.Filenames:
			if !ok {
				log(LogCatgCompletion, "CommandCompletionJob: filenames closed\n")
				filenamesClosed = true
				load.Filenames = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "CommandCompletionJob: got more filenames\n")
			fnames = append(fnames, x...)

		case x, ok := <-load.Errs:
			if !ok {
				log(LogCatgCompletion, "CommandCompletionJob: errors closed\n")
				errsClosed = true
				load.Errs = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "CommandCompletionJob: got error %v\n", x)
			err = x
		}
	}

	return
}

var pathListSeparatorString = string(os.PathListSeparator)

func splitPath(path string) []string {
	return strings.Split(path, pathListSeparatorString)
}

func (j CommandCompletionJob) Kill() {
	//j.load.Kill <- struct{}{}
}

func (j CommandCompletionJob) Name() string {
	return "command-completion"
}

func computeDirForCommandCompletion(completer *PathCompleter) (dir string, err error) {
	gpath := completer.CompleteNoCheck(".")

	dir = gpath.Dir().String()
	return

}

type stringSet map[string]struct{}

func (set stringSet) Add(s []string) {
	for _, n := range s {
		set[n] = struct{}{}
	}
}

func (set stringSet) Slice() []string {
	r := make([]string, len(set))
	i := 0
	for k := range set {
		r[i] = k
		i++
	}
	return r
}

var windowsExecutableExtensions = map[string]struct{}{
	// See https://aerorock.co.nz/list-of-executable-file-extensions-windows/
	"vb":       struct{}{},
	"ws":       struct{}{},
	"bat":      struct{}{},
	"bin":      struct{}{},
	"cmd":      struct{}{},
	"com":      struct{}{},
	"cpl":      struct{}{},
	"exe":      struct{}{},
	"ins":      struct{}{},
	"inx":      struct{}{},
	"isu":      struct{}{},
	"job":      struct{}{},
	"jse":      struct{}{},
	"lnk":      struct{}{},
	"msc":      struct{}{},
	"msi":      struct{}{},
	"msp":      struct{}{},
	"mst":      struct{}{},
	"paf":      struct{}{},
	"pif":      struct{}{},
	"ps1":      struct{}{},
	"reg":      struct{}{},
	"rgs":      struct{}{},
	"scr":      struct{}{},
	"sct":      struct{}{},
	"shb":      struct{}{},
	"shs":      struct{}{},
	"u3p":      struct{}{},
	"vbe":      struct{}{},
	"vbs":      struct{}{},
	"wsf":      struct{}{},
	"wsh":      struct{}{},
	"inf1":     struct{}{},
	"gadget":   struct{}{},
	"vbscript": struct{}{},
}

func removeNonExecutables(cmds []string) []string {
	if runtime.GOOS != "windows" {
		return cmds
	}

	r := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if isWindowsExecutable(c) {
			r = append(r, c)
		}
	}
	return r
}

func isWindowsExecutable(fname string) bool {
	n := strings.ToLower(fname)
	i := strings.IndexRune(fname, '.')
	if i < 0 {
		return false
	}

	ext := n[i+1:]
	_, ok := windowsExecutableExtensions[ext]
	return ok
}
