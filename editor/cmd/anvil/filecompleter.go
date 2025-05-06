package main

import (
	"path"
	"path/filepath"
	"strings"
)

// FilenameCompletions returns any paths in `dir` that would complete the partial filename `base`.
// The parameter `word` is the part of the filename the user typed so far in the editable, and is passed
// back to the editable when printing completions in order to determine how much of the completion we can
// append to the word in the editable.
func FilenameCompletionsAsync(word, dir, base string, callback CompletionsCallback) error {
	var ldr FileLoader
	load, err := ldr.LoadAsync(dir)
	if err != nil {
		return err
	}

	j := FilenameCompletionJob{
		load:     load,
		dir:      dir,
		word:     word,
		base:     base,
		work:     editor.WorkChan(),
		callback: callback,
	}

	editor.AddJob(&j)
	go j.run()
	return nil
}

type FilenameCompletionJob struct {
	load            *DataLoad
	dir             string
	word            string
	base            string
	filenamesClosed bool
	errsClosed      bool
	work            chan Work
	callback        CompletionsCallback
}

type CompletionsCallback func(completions []string)

func (j *FilenameCompletionJob) run() {
	errsClosed := false
	filenamesClosed := false
	contentsClosed := false

	done := func() bool {
		return (errsClosed && filenamesClosed) || (errsClosed && contentsClosed)
	}

	var fnames []string
	var err error

FOR:
	for {
		log(LogCatgCompletion, "FilenameCompletionJob: select\n")
		select {
		case _, ok := <-j.load.Contents:
			if !ok {
				contentsClosed = true
				j.load.Contents = nil
				if done() {
					break FOR
				}
				break
			}

		case x, ok := <-j.load.Filenames:
			if !ok {
				log(LogCatgCompletion, "FilenameCompletionJob: filenames closed\n")
				filenamesClosed = true
				j.load.Filenames = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "FilenameCompletionJob: got more filenames\n")
			fnames = append(fnames, x...)

		case x, ok := <-j.load.Errs:
			if !ok {
				log(LogCatgCompletion, "FilenameCompletionJob: errors closed\n")
				errsClosed = true
				j.load.Errs = nil
				if done() {
					break FOR
				}
				break
			}
			log(LogCatgCompletion, "FilenameCompletionJob: got error %v\n", x)
			err = x
		}
	}

	if err != nil {
		j.work <- &appendError{job: j, dir: j.dir, err: err}
		return
	}

	fnames = j.filesStartingWithBase(fnames)
	j.stripBase(fnames)
	j.prependWord(fnames)

	j.work <- &applyFilenameCompletionsToEditable{job: j,
		completions: fnames,
		word:        j.word,
		callback:    j.callback,
	}
}

func (j FilenameCompletionJob) filesStartingWithBase(fnames []string) []string {
	r := make([]string, 0, len(fnames))
	for _, n := range fnames {
		if strings.HasPrefix(n, j.base) {
			r = append(r, n)
		}
	}
	return r
}

func (j FilenameCompletionJob) stripBase(fnames []string) {
	for i, n := range fnames {
		if len(n) >= len(j.base) {
			fnames[i] = n[len(j.base):]
		}
	}
}

func (j FilenameCompletionJob) prependDir(fnames []string) {
	for i, n := range fnames {
		fnames[i] = filepath.Join(j.dir, n)
	}
}

func (j FilenameCompletionJob) prependWord(fnames []string) {
	for i, n := range fnames {
		fnames[i] = j.word + n
	}
}

func (j FilenameCompletionJob) Kill() {
	j.load.Kill <- struct{}{}
}

func (j FilenameCompletionJob) Name() string {
	return "file-completion"
}

type applyFilenameCompletionsToEditable struct {
	job         Job
	completions []string
	word        string
	callback    CompletionsCallback
}

func (l applyFilenameCompletionsToEditable) Service() (done bool) {
	l.callback(l.completions)
	return true
}

func (l applyFilenameCompletionsToEditable) Job() Job {
	return l.job
}

type appendError struct {
	job Job
	dir string
	err error
}

func (l appendError) Service() (done bool) {
	log(LogCatgCompletion, "FilenameCompletionJob: append error.Service: appending error %v\n", l.err)
	editor.AppendError(l.dir, l.err.Error())
	return true
}

func (l appendError) Job() Job {
	return l.job
}

func computeDirAndBaseForFilenameCompletion(fpath string, completer *PathCompleter) (dir, base string, err error) {
	gpath := completer.CompleteNoCheck(fpath)

	baseFn := func(p string) string {
		if len(p) > 0 && p[len(p)-1] == filepath.Separator {
			// Special case. If the path ends in the path separator, consider the base to be
			// the empty string.
			return ""
		}

		fn := filepath.Base
		if gpath.IsRemote() {
			fn = path.Base
		}

		return fn(p)
	}

	dir = gpath.Dir().String()
	base = baseFn(gpath.Path())
	return

}
