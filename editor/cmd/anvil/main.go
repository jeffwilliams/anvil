package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"

	_ "embed"
	//"net/http"

	"gioui.org/font"
	"github.com/jeffwilliams/anvil/editor/internal/ansi"
	adebug "github.com/jeffwilliams/anvil/editor/internal/debug"
	"github.com/jeffwilliams/anvil/editor/internal/expr"
	"github.com/jeffwilliams/anvil/editor/internal/typeset"
	"github.com/ogier/pflag"
)

const editorName = "anvil"

var optProfile = pflag.BoolP("profile", "p", false, "Profile the code CPU usage. The profile file location is printed to stdout.")
var optLoadDumpfile = pflag.StringP("load", "l", "", "Load state from the specified file that was created using Dump")
var optChdir = pflag.StringP("cd", "d", "", "Change directory to the specified path before starting")
var optDebugStdout = pflag.BoolP("dbg", "b", false, "Print debug logs to stdout")
var optPixelSizeFonts = pflag.BoolP("fonts-in-pixels", "f", false, "Consider font sizes in pixels instead of device independent units")
var optVersion = pflag.BoolP("version", "", false, "Print version")

func main() {
	parseAndValidateOptions()

	if *optVersion {
		fmt.Printf("%s %s\n", buildVersion, buildTime)
		return
	}

	if *optChdir != "" {
		err := os.Chdir(*optChdir)
		if err != nil {
			fmt.Printf("chdir failed: %v\n", err)
		}
	}

	if *optProfile {
		startProfiling(ProfileCPU)
	}

	printSyntaxLexerParseErrors()

	LoadSettings()
	LoadStyle()
	HirePlumber()
	ansi.InitColors(WindowStyle.Ansi.AsColors())
	application = NewApplication()
	editor = NewEditor(WindowStyle)

	LoadSshKeys()
	initDebugging()

	go ServeLocalAPI()

	var w app.Window
	application.SetWindow(&w)

	editorInitParams.dumpfileToLoad = *optLoadDumpfile
	editorInitParams.initialFiles = pflag.Args()

	loop(&w)

	if *optProfile {
		stopProfiling()
	}
}

func parseAndValidateOptions() {
	pflag.Parse()

	if pflag.NArg() > 0 && *optLoadDumpfile != "" {
		fmt.Printf("Filenames cannot be specified as arguments when the option --load is used\n")
		Exit(1)
	}
}

var styleLoadedFromFile bool

func LoadStyle() {
	style, err := LoadStyleFromConfigFile(&WindowStyle)
	if err != nil {
		log(LogCatgApp, "Loading style from config file failed: %v", err)
		return
	}

	log(LogCatgApp, "Loaded style from config file %s\n", StyleConfigFile())
	WindowStyle = style
	styleLoadedFromFile = true
}

var settingsLoadedFromFile bool
var settings = Settings{
	Ssh: SshSettings{
		Shell:             "sh",
		CacheSize:         5,
		CloseStdin:        false,
		ConnectionTimeout: 5,
	},
	Layout: LayoutSettings{
		EditorTag:         "Newcol Kill Putall Dump Load Exit Help ◊",
		ColumnTag:         "New Cut Paste Snarf Zerox Delcol",
		WindowTagUserArea: " Do Look ",
	},
}

func LoadSettings() {
	var err error
	err = LoadSettingsFromConfigFile(&settings)
	if err != nil {
		log(LogCatgApp, "Loading settings from config file failed: %v\n", err)
		return
	}

	log(LogCatgApp, "Loaded settings from config file %s\n", SettingsConfigFile())

	settingsLoadedFromFile = true
}

var plumbingLoadedFromFile bool

func HirePlumber() {
	HirePlumberUsingFile(PlumbingConfigFile())
}

func HirePlumberUsingFile(path string) error {
	rules, err := LoadPlumbingRulesFromFile(path)
	if err != nil {
		log(LogCatgApp, "Loading plumbing rules from config file failed: %v\n", err)
		return err
	}

	log(LogCatgApp, "Loaded plumbing rules from config file %s\n", PlumbingConfigFile())
	plumber = NewPlumber(rules)
	plumbingLoadedFromFile = true
	return nil
}

// https://colorhunt.co/palette/1624471f40681b1b2fe43f5a
var WindowStyle = Style{
	Fonts: []FontStyle{
		{
			FontName: "defaultVariableFont",
			FontFace: VariableFont,
			FontSize: 14,
		},
		{
			FontName: "defaultMonoFont",
			FontFace: MonoFont,
			FontSize: 14,
		},
	},
	TagFgColor:                      MustParseHexColor("#f0f0f0"),
	TagBgColor:                      MustParseHexColor("#263859"),
	TagPathBasenameColor:            MustParseHexColor("#f4a660"),
	BodyFgColor:                     MustParseHexColor("#f0f0f0"),
	BodyBgColor:                     MustParseHexColor("#17223B"),
	LayoutBoxFgColor:                MustParseHexColor("#9b2226"),
	LayoutBoxUnsavedBgColor:         MustParseHexColor("#9b2226"),
	LayoutBoxBgColor:                MustParseHexColor("#6B778D"),
	ScrollFgColor:                   MustParseHexColor("#17223B"),
	ScrollBgColor:                   MustParseHexColor("#6B778D"),
	WinBorderColor:                  MustParseHexColor("#000000"),
	WinBorderWidth:                  2,
	GutterWidth:                     14,
	PrimarySelectionFgColor:         MustParseHexColor("#17223B"),
	PrimarySelectionBgColor:         MustParseHexColor("#b1b695"),
	ExecutionSelectionFgColor:       MustParseHexColor("#17223B"),
	ExecutionSelectionBgColor:       MustParseHexColor("#fa8072"),
	SecondarySelectionFgColor:       MustParseHexColor("#17223B"),
	SecondarySelectionBgColor:       MustParseHexColor("#fcd0a1"),
	ErrorsTagFgColor:                MustParseHexColor("#f0f0f0"),
	ErrorsTagBgColor:                MustParseHexColor("#54494C"),
	ErrorsTagPathBasenameColor:      MustParseHexColor("#f4a660"),
	ErrorsTagFlashFgColor:           MustParseHexColor("#f0f0f0"),
	ErrorsTagFlashBgColor:           MustParseHexColor("#9b2226"),
	ErrorsTagFlashPathBasenameColor: MustParseHexColor("#f4a660"),
	TabStopInterval:                 30, // in pixels
	TabStopPadding:                  10, // in pixels
	LineSpacing:                     0,
	TextLeftPadding:                 3,
	Syntax: SyntaxStyle{
		// Colors borrowed from vim jellybeans color scheme https://github.com/nanotech/jellybeans.vim/blob/master/colors/jellybeans.vim
		KeywordColor:      MustParseHexColor("#8fbfdc"), // jellybeans color for PreProc
		NameColor:         MustParseHexColor("#f0f0f0"), // Color names as normal text.
		StringColor:       MustParseHexColor("#99ad6a"),
		NumberColor:       MustParseHexColor("#cf6a4c"), // jellybeans Constant
		OperatorColor:     MustParseHexColor("#f0f0f0"), // Color operators as normal text
		CommentColor:      MustParseHexColor("#888888"),
		PreprocessorColor: MustParseHexColor("#c6b6ee"), // jellybeans identifier
		HeadingColor:      MustParseHexColor("#99ad6a"),
		SubheadingColor:   MustParseHexColor("#c6b6ee"),
		//InsertedColor:     MustParseHexColor("#aa3939"),
		//DeletedColor:      MustParseHexColor("#2d882d"),
		InsertedColor: MustParseHexColor("#51a151"),
		DeletedColor:  MustParseHexColor("#ca6565"),
	},
	Ansi: AnsiStyle{
		Colors: [16]Color{
			MustParseHexColor("#000000"),
			MustParseHexColor("#800000"),
			MustParseHexColor("#008000"),
			MustParseHexColor("#808000"),
			MustParseHexColor("#000080"),
			MustParseHexColor("#800080"),
			MustParseHexColor("#008080"),
			MustParseHexColor("#c0c0c0"),
			MustParseHexColor("#808080"),
			MustParseHexColor("#ff0000"),
			MustParseHexColor("#00ff00"),
			MustParseHexColor("#ffff00"),
			MustParseHexColor("#0000ff"),
			MustParseHexColor("#ff00ff"),
			MustParseHexColor("#00ffff"),
			MustParseHexColor("#ffffff"),
		},
	},
}

var (
	editor      *Editor
	application *Application
	appWindow   *app.Window
	window      *Window
	plumber     *Plumber
	debugLog    *adebug.DebugLog = adebug.New(100)
)

func dumpPanic(i interface{}) {
	fname := fmt.Sprintf("%s.panic", editorName)
	f, err := os.Create(fname)
	if err != nil {
		fmt.Printf("Opening file '%s' failed: %v\n", fname, err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "panic: %v\n", i)
	fmt.Fprintf(f, "%s", string(debug.Stack()))
}

func dumpLogs() {
	fname := fmt.Sprintf("%s.panic-logs", editorName)
	f, err := os.Create(fname)
	if err != nil {
		fmt.Printf("Opening file '%s' failed: %v\n", fname, err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, debugLog.String())
}

func dumpGoroutines() {
	fname := fmt.Sprintf("%s.panic-gortns", editorName)

	f, err := os.Create(fname)
	if err != nil {
		fmt.Printf("Opening file '%s' failed: %v\n", fname, err)
		return
	}
	defer f.Close()

	buf := make([]byte, 100000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]

	fmt.Fprintf(f, string(buf))
}

func initializeEditorToCurrentDirectory() {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	col = editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	window := col.NewWindow()
	wd := Getwd()
	gwd := NewGlobalPath(wd, GlobalPathIsDir)
	window.LoadFile(gwd, gwd)
}

func initializeEditorToFiles(files []string) {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	completer := NewPathCompleter()
	for _, f := range files {
		loadPath, displayPath := completeDisplayAndLoadPaths(completer, f)

		editor.LoadFile(displayPath, loadPath)
	}
}

func initializeEditorWithDumpfile(f string) {
	var state ApplicationState
	err := ReadState(f, &state)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Load failed: %v", err))
		return
	}
	application.SetState(&state)
}

func loop(w *app.Window) {
	defer func() {
		if r := recover(); r != nil {
			dumpPanic(r)
			dumpLogs()
			dumpGoroutines()
			panic(r)
		}
	}()

	appWindow = w

	events := make(chan event.Event)
	acks := make(chan struct{})
	go func() {
		for {
			ev := w.Event()
			events <- ev
			<-acks
			if _, ok := ev.(app.DestroyEvent); ok {
				return
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dumpPanic(r)
				dumpLogs()
				dumpGoroutines()
				panic(r)
			}
		}()

		for {
			select {
			case e := <-events:
				handleEvent(e)
				acks <- struct{}{}
			case w := <-editor.WorkChan():
				done := w.Service()
				if done && w.Job() != nil {
					editor.RemoveJob(w.Job())
					if sn, ok := w.Job().(StartNexter); ok {
						sn.StartNext()
					}
				}

				appWindow.Invalidate()
			}
		}
	}()

	app.Main()
}

var focusSet bool

func handleEvent(e event.Event) {
	var ops op.Ops
	switch e := e.(type) {
	case app.DestroyEvent:
		Exit(0)
	case app.FrameEvent:
		application.SetMetric(e.Metric)

		// In some places we need the metrics for determining the size of
		// widgets, so we delay creating the widgets until we have metrics.
		initializeEditorIfNeeded()

		gtx := app.NewContext(&ops, e)
		layoutWidgets(gtx)

		if !focusSet && window != nil {
			window.SetFocus(gtx)
			focusSet = true
			window = nil
		}

		e.Frame(gtx.Ops)
	case app.ConfigEvent:
		log(LogCatgUI, "window config changed: %v\n", e.Config)
		application.WindowConfigChanged(&e.Config)
	}
}

var editorInitParams = struct {
	dumpfileToLoad string
	initialFiles   []string
}{}
var editorInitialized = false

func initializeEditorIfNeeded() {
	if editorInitialized {
		return
	}

	editorInitialized = true

	application.SetTitle(editorName)

	if editorInitParams.dumpfileToLoad != "" {
		initializeEditorWithDumpfile(editorInitParams.dumpfileToLoad)
	} else if len(editorInitParams.initialFiles) > 0 {
		initializeEditorToFiles(editorInitParams.initialFiles)
	} else {
		initializeEditorToCurrentDirectory()
	}

	executeStartupCommands()
}

//go:embed font/InputMonoCondensed-ExtraLight.ttf
var InputMonoFont []byte

//go:embed font/InputSansCondensed-ExtraLight.ttf
var InputVariableFont []byte

// Set the default font to the Input font
var MonoFont = text.FontFace{
	Font: font.Font{
		Typeface: "defaultMonoFont",
	},
	Face: MustParseTTFBytes(InputMonoFont),
	// Uncomment the below to make the default font the Go fonts.
	//Face: MustParseTTFBytes(gomono.TTF),
}

var VariableFont = text.FontFace{
	Font: font.Font{
		Typeface: "defaultVariableFont",
	},
	Face: MustParseTTFBytes(InputVariableFont),
	// Uncomment the below to make the default font the Go fonts.
	//Face: MustParseTTFBytes(goregular.TTF),
}

func MustParseTTFBytes(b []byte) font.Face {
	face, err := typeset.ParseTTFBytes(b)
	if err != nil {
		panic(err.Error())
	}
	return face
}

type Collection []text.FontFace

func (c Collection) ContainsFont(font font.Font) bool {
	for _, f := range c {
		if f.Font == font {
			return true
		}
	}
	return false
}

func layoutWidgets(gtx layout.Context) {
	editor.Layout(gtx)
}

func Exit(code int) {
	if *optProfile {
		stopProfiling()
	}
	os.Exit(code)
}

func init() {
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [file]\n", os.Args[0])
		fmt.Printf("Launch the Anvil text editor. If [file] is given, that file is opened.\n\n")
		fmt.Printf("Options:\n")

		pflag.PrintDefaults()
	}
}

type EditorInitializationParams struct {
	LoadDumpFile string
	InitialFile  string
}

func log(category, message string, args ...interface{}) {
	if *optDebugStdout {
		fmt.Printf(message, args...)
	}
	debugLog.Addf(category, message, args...)
}

func initDebugging() {
	expr.Debug = func(message string, args ...interface{}) {
		log(LogCatgExpr, message, args...)
	}
}

func executeStartupCommands() {
	go func() {
		for _, cmd := range settings.General.ExecuteOnStartup {

			fn := func() {
				lcmd := cmd
				log(LogCatgApp, "executeStartupCommands: adding command '%s' in context of editor for next layout\n", lcmd)
				editor.Tag.AddOpForNextLayout(func(gtx layout.Context) {
					log(LogCatgApp, "executeStartupCommands: running command '%s' in context of editor\n", lcmd)
					editor.Tag.adapter.execute(&editor.Tag.blockEditable.editable, gtx, lcmd, nil)
				})
			}

			editor.WorkChan() <- basicWork{fn}
		}
	}()
}

//func setCursor(name string) {
/*
		List of cursor names on X11:
		https://www.oreilly.com/library/view/x-window-system/9780937175149/ChapterD.html
	X_cursor 	0
	arrow 	2
	based_arrow_down 	4
	based_arrow_up 	6
	boat 	8
	bogosity 	10
	bottom_left_corner 	12
	bottom_right_corner 	14
	bottom_side 	16
	bottom_tee 	18
	box_spiral 	20
	center_ptr 	22
	circle 	24
	clock 	26
	coffee_mug 	28
	cross 	30
	cross_reverse 	32
	crosshair 	34
	diamond_cross 	36
	dot 	38
	dotbox 	40
	double_arrow 	42
	draft_large 	44
	draft_small 	46
	draped_box 	48
	exchange 	50
	fleur 	52
	gobbler 	54
	gumby 	56
	hand1 	58
	hand2 	60
	heart 	62
	icon 	64
	iron_cross 	66
	left_ptr 	68
	left_side 	70
	left_tee 	72
	leftbutton 	74
	ll_angle 	76
	lr_angle 	78
	man 	80
	middlebutton 	82
	mouse 	84
	pencil 	86
	pirate 	88
	plus 	90
	question_arrow 	92
	right_ptr 	94
	right_side 	96
	right_tee 	98
	rightbutton 	100
	rtl_logo 	102
	sailboat 	104
	sb_down_arrow 	106
	sb_h_double_arrow 	108
	sb_left_arrow 	110
	sb_right_arrow 	112
	sb_up_arrow 	114
	sb_v_double_arrow 	116
	shuttle 	118
	sizing 	120
	spider 	122
	spraycan 	124
	star 	126
	target 	128
	tcross 	130
	top_left_arrow 	132
	top_left_corner 	134
	top_right_corner 	136
	top_side 	138
	top_tee 	140
	trek 	142
	ul_angle 	144
	umbrella 	146
	ur_angle 	148
	watch 	150
	xterm
*/
/*
	if appWindow != nil {
		//w.SetCursorName("icon")
		appWindow.SetCursorName(pointer.CursorName(name))
	}

}
*/
