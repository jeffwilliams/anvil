package main

import (
	"time"

	"gioui.org/layout"
	"github.com/jeffwilliams/anvil/editor/internal/words"
)

type Body struct {
	blockEditable
	syntaxStyle SyntaxStyle
}

func (b *Body) Init(style blockStyle, editableStyle editableStyle, syntaxStyle SyntaxStyle, executor *CommandExecutor, owner interface{}, workChan chan Work) {

	scheduler := NewScheduler(workChan)
	b.blockEditable.Init(style, editableStyle, scheduler)
	b.executeOn = &b.editable
	b.syntaxStyle = syntaxStyle
	b.colorizeAnsiEscapes = true
	b.SetAdapter(&editableAdapter{
		executor: executor,
		owner:    owner,
		creator:  b,
	})
}

func (b *Body) EnableSyntax(filename string) {
	b.syntaxHighlighter = NewSyntaxHighlighter(b.syntaxStyle)
	b.asyncHighlighter = NewAsyncHighlighter(b.syntaxHighlighter, 100*time.Millisecond, b.asyncSyntaxHighlightingDone)
	b.syntaxHighlighter.SetFilename(filename)
}

func (b *Body) EnableCompletion() {
	b.completer = words.NewCompleter()
}

func (b *Body) SetSyntaxLanguage(lang string) {
	if b.syntaxHighlighter == nil {
		b.EnableSyntax("")
	}
	b.syntaxHighlighter.SetLanguage(lang)
}

func (b *Body) SetSyntaxAnalyse(v bool) {
	if b.syntaxHighlighter == nil {
		b.EnableSyntax("")
	}

	if a, ok := b.syntaxHighlighter.(AnalyzingHighlighter); ok {
		a.SetAnalyse(v)
	}
}

func (b *Body) DisableSyntax() {
	b.syntaxHighlighter = nil
}

func (b *Body) layout(gtx layout.Context) layout.Dimensions {
	b.dimensionBehaviours.width = expand
	b.dimensionBehaviours.height = expand
	return b.blockEditable.layout(gtx)
}

func (b *Body) SetStyle(style blockStyle, editableStyle editableStyle, syntaxStyle SyntaxStyle) {
	b.blockEditable.SetStyle(style, editableStyle)
	b.syntaxStyle = syntaxStyle
	if b.syntaxHighlighter != nil {
		b.syntaxHighlighter.SetStyle(syntaxStyle)
		b.HighlightSyntax()
	}
}
