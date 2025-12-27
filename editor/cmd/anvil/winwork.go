package main

import (
	"gioui.org/layout"
)

type WindowDataLoad struct {
	DataLoad
	Jobname string
	Win     WindowHolder
	Opts    LoadFileOpts
	Job     Job
}

type WindowHolder struct {
	win     *Window
	winName string
}

func (h *WindowHolder) Get() *Window {
	if h.win != nil {
		return h.win
	}
	h.win = editor.FindOrCreateWindow(h.winName)
	return h.win
}

func (h *WindowHolder) LoadByName() bool {
	return h.winName != ""
}

func NewWindowHolder(win *Window) WindowHolder {
	return WindowHolder{win: win}
}

func NewWindowHolderForName(winName string) WindowHolder {
	return WindowHolder{winName: winName}
}

func (f *WindowDataLoad) Start(c chan Work) {
	go f.pump(c)
}

func (f *WindowDataLoad) GetJob() Job {
	if f.Job == nil {
		return f
	}
	return f.Job
}

type WindowDataLoadSender struct {
	contentsClosed  bool
	filenamesClosed bool
	errsClosed      bool
	sentType        bool
	work            chan Work
	load            *WindowDataLoad
}

func (w WindowDataLoadSender) workIsDone() bool {
	return (w.contentsClosed && w.errsClosed) || (w.filenamesClosed && w.errsClosed)
}

func (w *WindowDataLoadSender) sendType(t fileType) {
	if w.sentType {
		return
	}

	w.work <- &winSetFiletype{job: w.load.GetJob(), win: w.load.Win, fileType: t}
	w.sentType = true
}

func (w *WindowDataLoadSender) updateStateWhenContentsClosed() {
	log(LogCatgWin, "pump: contents is closed\n")
	w.contentsClosed = true
	w.load.Contents = nil
}

func (w *WindowDataLoadSender) sendContents(x []byte) {
	w.sendType(typeFile)

	log(LogCatgWin, "pump: got some contents\n")
	w.work <- &winLoadData{job: w.load.GetJob(), win: w.load.Win, data: x, growBodyBehaviour: w.load.Opts.GrowBodyBehaviour}
	if w.load.Opts.Tail {
		w.work <- &winLoadGoToEnd{job: w.load.GetJob(), win: w.load.Win}
	}
}

func (w *WindowDataLoadSender) updateStateWhenFilenamesClosed() {
	log(LogCatgWin, "pump: contents is closed\n")
	w.filenamesClosed = true
	w.load.Filenames = nil
}

func (w *WindowDataLoadSender) sendFilenames(x []string) {
	w.sendType(typeDir)
	w.work <- &winLoadNames{job: w.load.GetJob(), win: w.load.Win, names: x}
	log(LogCatgWin, "pump: got some filenames\n")
}

func (w *WindowDataLoadSender) updateStateWhenErrorsClosed() {
	log(LogCatgWin, "pump: errors is closed\n")
	w.errsClosed = true
	w.load.Errs = nil
}

func (w *WindowDataLoadSender) sendError(x error) {
	log(LogCatgWin, "pump: got an error: %v %T\n", x, x)
	w.work <- &winLoadErr{job: w.load.GetJob(), win: w.load.Win, err: x}
}

func (w *WindowDataLoadSender) finalize() {
	// If we are writing this to an existing errors window, don't do any of the normal finalization actions,
	// just signify that the job is complete. This is to prevent popping up an empty errors window
	if w.load.Win.LoadByName() {
		w.work <- &winLoadDone{job: w.load.GetJob(), win: w.load.Win, selectBehaviour: w.load.Opts.SelectBehaviour}
		return
	}

	// In case there is no such file (like in the case of a New for a non-existent file),
	// set the window to be a file
	w.sendType(typeFile)

	log(LogCatgWin, "pump done\n")
	w.work <- &winLoadDone{job: w.load.GetJob(), win: w.load.Win, goTo: w.load.Opts.GoTo, selectBehaviour: w.load.Opts.SelectBehaviour}
	close(w.load.DataLoad.Kill)
}

func (f *WindowDataLoad) pump(c chan Work) {
	log(LogCatgWin, "pump started\n")

	sender := WindowDataLoadSender{
		work: c,
		load: f,
	}

FOR:
	for {
		select {
		case x, ok := <-f.Contents:
			if !ok {
				sender.updateStateWhenContentsClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendContents(x)
		case x, ok := <-f.Filenames:
			if !ok {
				sender.updateStateWhenFilenamesClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendFilenames(x)
		case x, ok := <-f.Errs:
			if !ok {
				sender.updateStateWhenErrorsClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendError(x)
		}
	}

	sender.finalize()
	log(LogCatgWin, "pump finished\n")
}

type growBodyBehaviour int

const (
	growBodyIfTooSmall = iota
	dontGrowBodyIfTooSmall
)

type moveLayerBehaviour int

const (
	moveToCurrentLayer = iota
	dontMoveToCurrentLayer
)

func (l *WindowDataLoad) Kill() {
	select {
	case l.DataLoad.Kill <- struct{}{}:
	default:
	}
}

func (l *WindowDataLoad) Name() string {
	return l.Jobname
}

// WindowDataChunk is a chunk of data to be written to a window, or an error
type winLoadData struct {
	job               Job
	win               WindowHolder
	data              []byte
	growBodyBehaviour growBodyBehaviour
}

type winLoadNames struct {
	job   Job
	win   WindowHolder
	names []string
}

type winLoadErr struct {
	job Job
	win WindowHolder
	err error
}

type winLoadDone struct {
	job             Job
	win             WindowHolder
	goTo            seek
	selectBehaviour selectBehaviour
}

type winLoadGoToEnd struct {
	job Job
	win WindowHolder
}

type winSetFiletype struct {
	job      Job
	win      WindowHolder
	fileType fileType
}

type Work interface {
	// Service returns true if this work completes the job, otherwise it returns false
	Service() (done bool)
	// Job returns which job this bit of work is for
	Job() Job
}

func (l winLoadData) Service() (done bool) {
	win := l.win.Get()
	if win == nil {
		return false
	}

	win.Append(l.data)
	if l.growBodyBehaviour == growBodyIfTooSmall {
		win.showIfHidden()
		editor.EnsureWindowIsInCurrentLayer(win)
		win.GrowIfBodyTooSmall()
		editor.SetOnlyFlashedWindow(win)
	}

	log(LogCatgWin, "Appended %d bytes to window %s\n", len(l.data), win.displayPath.String())
	return false
}

func (l winLoadData) Job() Job {
	return l.job
}

func (l winLoadNames) Service() (done bool) {
	win := l.win.Get()
	win.filler.AppendItems(l.names)
	win.SetTagFromDisplayPath()
	return false
}

func (l winLoadNames) Job() Job {
	return l.job
}

func (l winLoadErr) Service() (done bool) {
	dir := ""
	win := l.win.Get()
	if win != nil {
		completer := NewPathCompleterForWindow(win)
		dir = completer.Dir().String()
	}
	editor.AppendError(dir, l.err.Error())
	return true
}

func (l winLoadErr) Job() Job {
	return l.job
}

func (l winLoadDone) Service() (done bool) {
	// If we are writing this to an existing errors window, don't do any of the normal finalization actions,
	// just signify that the job is complete. This is to prevent popping up an empty errors window
	if l.win.LoadByName() {
		return true
	}

	win := l.win.Get()
	if win != nil {
		win.markTextAsUnchanged()
		win.SetTagFromDisplayPath()
		win.Body.AddOpForNextLayout(func(gtx layout.Context) {
			// This is to force a redraw
			win.Body.invalidateLayedoutText()
		})
		if !l.goTo.empty() {
			log(LogCatgWin, "winLoadDone: adding a goto for next layout. Goto: %v\n", l.goTo)
			win.Body.AddOpForNextLayout(func(gtx layout.Context) {
				log(LogCatgWin, "editable next layout op called: goto %v\n", l.goTo)
				win.Body.moveCursorTo(gtx, l.goTo, l.selectBehaviour)
			})
		}
		win.maybeEnableSyntax()
	}
	return true
}

func (l winLoadDone) Job() Job {
	return l.job
}

func (l winLoadGoToEnd) Service() (done bool) {
	win := l.win.Get()
	if win == nil {
		return false
	}
	win.Body.AddOpForNextLayout(func(gtx layout.Context) {
		win.Body.moveToEndOfDoc(gtx)
	})
	return false
}

func (l winLoadGoToEnd) Job() Job {
	return l.job
}

func (l winSetFiletype) Service() (done bool) {
	win := l.win.Get()
	if win == nil {
		return false
	}

	win.loadPath.SetDirState(GlobalPathDirState(l.fileType))
	win.displayPath.SetDirState(GlobalPathDirState(l.fileType))
	win.SetPathsAndTag(&win.displayPath, &win.loadPath)

	if l.fileType == typeDir {
		win.filler = NewFillEditableWithItemList(&win.Body.layouter, &win.layout.style, []string{})
		win.Body.SetPreDrawHook(win.filler.preDrawHook)
	} else {
		win.Body.SetPreDrawHook(nil)
	}

	return false
}

func (l winSetFiletype) Job() Job {
	return l.job
}

type WindowDataSave struct {
	Jobname string
	Win     *Window
	errs    chan error
	kill    chan struct{}
}

func (s WindowDataSave) Name() string {
	return s.Jobname
}

func (s WindowDataSave) Kill() {
	select {
	case s.kill <- struct{}{}:
	default:
	}
}

func (s *WindowDataSave) Start(c chan Work) {
	go s.wait(c)
}

func (s *WindowDataSave) wait(c chan Work) {
	defer close(s.kill)

	e, ok := <-s.errs
	if !ok {
		// errors closed
		c <- &winSaveDone{job: s, win: s.Win}
		s.Win.notifyPut()
		return
	}
	c <- &winLoadErr{job: s, err: e}
	s.Win.notifyPut()
}

type winSaveDone struct {
	job Job
	win *Window
}

func (l winSaveDone) Service() (done bool) {
	l.win.markTextAsUnchanged()
	l.win.SetTagFromDisplayPath()
	return true
}

func (l winSaveDone) Job() Job {
	return l.job
}

type emptyNamedWork struct {
	name string
	done bool
}

func (w emptyNamedWork) Service() bool {
	return w.done
}

func (w emptyNamedWork) Job() Job {
	return w
}

func (w emptyNamedWork) Kill() {
}

func (w emptyNamedWork) Name() string {
	return w.name
}
