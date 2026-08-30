package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"time"

	"gioui.org/app"
)

type ApplicationState struct {
	Title            string
	Editor           *EditorState
	AppWindowCfgSet  bool
	AppWindowSize    image.Point
	AppWindowMode    app.WindowMode
	WinIdGenState    *IdGenState
	CommandHistory   *CommandHistoryState
	WorkingDirectory string
	Trays            []TrayState
}

func (a *Application) State() *ApplicationState {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}

	s := &ApplicationState{
		Title:            application.appWindowTitle,
		Editor:           editor.State(),
		WinIdGenState:    a.WinIdGenerator().State(),
		CommandHistory:   cmdHistory.State(),
		WorkingDirectory: wd,
		Trays:            sessionScopedTrays.State(),
	}

	if a.winConfig != nil {
		s.AppWindowCfgSet = true
		s.AppWindowSize = a.winConfig.Size
		s.AppWindowMode = a.winConfig.Mode
	}

	return s
}

func (a *Application) SetState(state *ApplicationState) error {
	if state == nil {
		return fmt.Errorf("The application state is nil")
	}

	if state.WorkingDirectory != "" {
		err := os.Chdir(state.WorkingDirectory)
		if err != nil {
			log(LogCatgApp, "Application.SetState: error setting working directory: %v\n", err)
		}
	}

	a.SetTitle(state.Title)
	a.WinIdGenerator().SetState(state.WinIdGenState)
	editor.SetState(state.Editor)

	if state.AppWindowCfgSet {
		if state.AppWindowMode == app.Windowed {
			a.SetWindowSize(state.AppWindowSize)
		}
	}

	// Preserve any commands run before Load was called
	h := NewCommandHistory(cmdHistory.max)
	h.SetState(state.CommandHistory)
	cmdHistory = cmdHistory.Merge(h)

	sessionScopedTrays.SetState(state.Trays)
	return nil
}

type EditorState struct {
	Tag              *TagState
	Layers           []*LayerState
	ActiveLayerIndex int
	RecentFiles      []string
	Marks            MarkState
}

func (e *Editor) State() *EditorState {
	edTag := e.Tag.State()

	var layers []*LayerState
	for _, l := range e.Layers {
		layers = append(layers, l.State())
	}

	// Remove any running jobs, since they won't be running after load.
	edTag.Text = e.removeJobsFromTag(edTag.Text)

	return &EditorState{
		Tag:              edTag,
		Layers:           layers,
		ActiveLayerIndex: editor.activeLayerIndex,
		RecentFiles:      editor.recentFiles.All(),
		Marks:            editor.Marks.State(),
	}

	//e.focusedEditable
}

func (e *Editor) removeJobsFromTag(tag string) string {
	for _, j := range e.jobs {
		tag, _, _ = removeJobFromTagString(j.Name(), tag)
	}
	return tag
}

func (e *Editor) addJobsToTag() {
	for _, j := range e.jobs {
		e.prependJobToTag(j)
	}
}

func (e *Editor) SetState(state *EditorState) error {
	if state == nil {
		return fmt.Errorf("The editor state is nil")
	}
	e.Tag.SetState(state.Tag)
	// Add back any jobs that were running before Load was executed
	editor.addJobsToTag()

	// Remove all columns
	//editor.Clear()

	editor.Layers = nil
	for _, layerState := range state.Layers {
		layer := editor.NewLayer()
		layer.SetState(layerState)
		editor.Layers = append(editor.Layers, layer)
	}

	editor.activeLayerIndex = state.ActiveLayerIndex

	for _, f := range state.RecentFiles {
		editor.AddRecentFile(f)
	}

	editor.Marks.SetState(state.Marks)

	return nil
}

type TagState struct {
	Text string
}

func (t *Tag) State() *TagState {
	return &TagState{Text: t.String()}
}

func (t *Tag) SetState(s *TagState) error {
	if s == nil {
		return fmt.Errorf("The tag state is nil")
	}
	t.SetTextStringNoUndo(s.Text)
	return nil
}

type LayerState struct {
	Name           string
	Cols           []*ColState
	VisibleCols    []VisibleColState
	LeftVisibleCol int
}

type VisibleColState struct {
	LeftX    int
	ColIndex int
}

func (l *Layer) State() *LayerState {
	var cols []*ColState
	for _, c := range l.Cols {
		cols = append(cols, c.State())
	}

	visibleCols := make([]VisibleColState, len(l.visibleCols))
	for i, c := range l.visibleCols {
		visibleCols[i].LeftX = c.leftX
		visibleCols[i].ColIndex = c.col.layedOutColIndex
	}

	return &LayerState{
		Name:           l.Name,
		Cols:           cols,
		VisibleCols:    visibleCols,
		LeftVisibleCol: l.leftVisibleCol,
	}
}

func (layer *Layer) SetState(state *LayerState) error {
	if state == nil {
		return fmt.Errorf("The layer state is nil")
	}

	layer.Name = state.Name

	// Remove all columns
	layer.Clear()

	for _, c := range state.Cols {
		col := layer.NewColDontPosition()
		col.SetState(c)
	}

	layer.leftVisibleCol = state.LeftVisibleCol

	layer.visibleCols = make([]colPosition, len(state.VisibleCols))
	for i, x := range state.VisibleCols {
		layer.visibleCols[i].leftX = x.LeftX
		layer.visibleCols[i].col = layer.Cols[x.ColIndex]
	}

	return nil
}

type ColState struct {
	Tag              *TagState
	LayedOutColIndex int
	Windows          []*WindowState
}

func (c *Col) State() *ColState {

	var wins []*WindowState
	for _, w := range c.Windows {
		wins = append(wins, w.State())
	}

	return &ColState{
		Tag:              c.Tag.State(),
		LayedOutColIndex: c.layedOutColIndex,
		Windows:          wins,
	}
}

func (c *Col) SetState(state *ColState) error {
	if state == nil {
		return fmt.Errorf("The column state is nil")
	}
	c.Tag.SetState(state.Tag)
	c.layedOutColIndex = state.LayedOutColIndex

	for _, w := range state.Windows {
		win := c.NewWindowDontPosition()
		win.SetState(w)
	}
	return nil
}

type WindowState struct {
	Tag                  *TagState
	TopY                 int
	Body                 *BodyState
	DisplayPath          string
	LoadPath             string
	FileType             fileType
	Id                   int
	CloneIds             []int
	ManualHighlighting   []ManualHighlightingInterval
	InsertWhenTabPressed string
	PinnedToLayer        bool
}

type ManualHighlightingInterval struct {
	Start, End int
	FgColor    Color
	BgColor    Color
}

func (w *Window) State() *WindowState {
	cloneIds := make([]int, len(w.clones))
	i := 0
	for c := range w.clones {
		cloneIds[i] = c.Id
		i++
	}

	attemptSavingContents := true
	if w.fileType == typeDir {
		attemptSavingContents = false
	}

	manualHighlighting := make([]ManualHighlightingInterval, len(w.Body.manualHighlighting))
	for i, v := range w.Body.manualHighlighting {
		manualHighlighting[i].Start = v.start
		manualHighlighting[i].End = v.end
		manualHighlighting[i].FgColor = v.fgColor
		manualHighlighting[i].BgColor = v.bgColor
	}

	return &WindowState{
		Tag:                  w.Tag.State(),
		TopY:                 w.TopY,
		Body:                 w.Body.State(attemptSavingContents),
		DisplayPath:          w.displayPath.String(),
		LoadPath:             w.loadPath.String(),
		FileType:             w.fileType,
		Id:                   w.Id,
		CloneIds:             cloneIds,
		ManualHighlighting:   manualHighlighting,
		InsertWhenTabPressed: w.insertWhenTabPressed,
		PinnedToLayer:        w.IsPinnedToCurrentLayer(),
	}
}

func (w *Window) SetState(state *WindowState) error {
	if state == nil {
		return fmt.Errorf("The window state is nil")
	}
	w.Tag.SetState(state.Tag)
	w.TopY = state.TopY
	w.initialTagUserArea = ""
	displayPath := NewGlobalPath(state.DisplayPath, GlobalPathDirState(state.FileType))
	loadPath := NewGlobalPath(state.LoadPath, GlobalPathDirState(state.FileType))
	w.SetPathsAndTag(displayPath, loadPath)
	w.Body.SetState(state.Body)
	if state.Body.Text == "" {
		w.GetWithSelect(dontSelectText, dontGrowBodyIfTooSmall)
	}

	w.Body.manualHighlighting = make([]*SyntaxInterval, len(state.ManualHighlighting))
	for i, v := range state.ManualHighlighting {
		w.Body.manualHighlighting[i] = NewSyntaxInterval(v.Start, v.End, v.FgColor, v.BgColor)
	}

	application.WinIdGenerator().Free(w.Id)
	w.Id = state.Id
	w.insertWhenTabPressed = state.InsertWhenTabPressed

	// The clone we are searching for may not have been loaded yet.
	// But as we load more windows from the state dump we will eventually
	// load all missing windows, and they will get linked bidirectionally.
	for _, id := range state.CloneIds {
		clone := editor.FindWindowForId(id)
		if clone != nil {
			w.addClone(clone)
			clone.addClone(w)
			w.Body.text = clone.Body.text
		}
	}

	w.SetPinnedToCurrentLayer(state.PinnedToLayer)

	return nil
}

type BodyState struct {
	CursorIndices    []int
	TopLeftIndex     int
	Text             string
	FontIndex        int
	BackgroundImage  string
	BgImgScalingType int
	BgImgFraction    float32
}

const MaxWindowBodyLenToDump = 4096

func (b *Body) State(attemptSavingContents bool) *BodyState {
	state := &BodyState{
		CursorIndices:    b.CursorIndices,
		TopLeftIndex:     b.TopLeftIndex,
		FontIndex:        b.curFontIndex,
		BackgroundImage:  b.bgimage.filename,
		BgImgScalingType: int(b.bgimage.scalingType),
		BgImgFraction:    b.bgimage.fraction,
	}

	if attemptSavingContents {
		if !b.text.IsMarked() {
			// Not saved
			str := b.String()
			if len(str) < MaxWindowBodyLenToDump {
				state.Text = str
			}
		}
	}

	return state
}

func (b *Body) SetState(state *BodyState) error {
	if state == nil {
		return fmt.Errorf("The body state is nil")
	}
	if state.Text != "" {
		b.SetTextString(state.Text)
	}
	b.CursorIndices = state.CursorIndices
	b.TopLeftIndex = state.TopLeftIndex
	b.curFontIndex = state.FontIndex

	var err error
	if state.BackgroundImage != "" {
		err = b.bgimage.Load(state.BackgroundImage)
		if err == nil {
			b.bgimage.scalingType = scalingType(state.BgImgScalingType)
			b.bgimage.fraction = state.BgImgFraction
		}
	}
	b.invalidateLayedoutText()
	b.initTextRenderer()
	return err
}

type IdGenState struct {
	Free []int
	Next int
}

func (g *IdGen) State() *IdGenState {
	return &IdGenState{
		Next: g.next,
		Free: g.free,
	}
}

func (g *IdGen) SetState(state *IdGenState) error {
	if state == nil {
		return fmt.Errorf("The id generator state is nil")
	}
	g.next = state.Next
	g.free = state.Free
	return nil
}

func WriteState(path string, state interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

func ReadState(path string, state interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewDecoder(file)
	return enc.Decode(state)
}

type CommandHistoryState struct {
	Cmds []CommandHistoryEntryState
}

type CommandHistoryEntryState struct {
	Cmd     string
	Started time.Time
	Ended   time.Time
	State   RunState
	Dir     string
}

func (c *CommandHistory) State() *CommandHistoryState {
	state := &CommandHistoryState{
		Cmds: []CommandHistoryEntryState{},
	}

	c.cmds.Each(func(v *CommandHistoryEntry) {
		log(LogCatgApp, "CommandHistory.State: found a cmd entry\n")
		st := CommandHistoryEntryState{
			Cmd:     v.cmd,
			Started: v.started,
			Ended:   v.ended,
			State:   v.state,
			Dir:     v.dir,
		}

		state.Cmds = append(state.Cmds, st)
	})

	return state
}

func (c *CommandHistory) SetState(state *CommandHistoryState) {
	if state == nil {
		return
	}

	for _, scmd := range state.Cmds {
		e := &CommandHistoryEntry{
			cmd:     scmd.Cmd,
			started: scmd.Started,
			ended:   scmd.Ended,
			state:   scmd.State,
			dir:     scmd.Dir,
		}

		if e.state == Running {
			e.state = Orphaned
		}

		c.cmds.Add(e)
	}
}

type TrayState struct {
	Name string
	Text string
}

func (r *Trays) State() []TrayState {
	state := []TrayState{}
	for k, f := range r.floats {
		state = append(state, TrayState{k, f.content.String()})
	}

	return state
}

func (r *Trays) SetState(state []TrayState) {
	if state == nil {
		return
	}

	for _, rs := range state {
		r.add(rs.Name, rs.Text)
	}
}
