package main

import (
	"image"

	"gioui.org/app"
	"gioui.org/unit"
)

// Application handles application-wide settings and changes.
type Application struct {
	appWindowTitle string
	appWindow      *app.Window
	winConfig      *app.Config
	metric         *unit.Metric
	winIdGenerator IdGen
	colIdGenerator IdGen
}

func (a *Application) SetWindow(appWindow *app.Window) {
	a.appWindow = appWindow
	if a.appWindowTitle != "" {
		a.SetTitle(a.appWindowTitle)
	}
}

func (a *Application) SetTitle(t string) {
	a.appWindowTitle = t
	if a.appWindow != nil {
		a.appWindow.Option(app.Title(t))
	}
}

func (a *Application) Title() string {
	return a.appWindowTitle
}

func (a *Application) SetMetric(metric unit.Metric) {
	a.metric = &unit.Metric{}
	*a.metric = metric
}

func (a *Application) Metric() *unit.Metric {
	return a.metric
}

func NewApplication() *Application {
	return &Application{}
}

func (a *Application) WindowConfigChanged(cfg *app.Config) {
	a.winConfig = &app.Config{}
	*a.winConfig = *cfg
}

func (a *Application) SetWindowSize(sz image.Point) {
	log(LogCatgApp, "Application: requested to set window size to %v\n", sz)
	if a.appWindow == nil {
		log(LogCatgApp, "Application: can't set window size because the window is not yet created\n")
		return
	}

	if a.metric == nil {
		log(LogCatgApp, "Application: can't set window size because the metric is not yet known\n")
		return
	}

	if sz.X == 0 || sz.Y == 0 {
		return
	}

	// In GIO, the window size reported in a app.Config from an app.ConfigEvent is in units of display dependent pixels.
	// However setting the window size is expected to be done using divice independent pixels. So we must convert here.
	pxPerDp := float32(1)
	if a.metric.PxPerDp > 0 {
		pxPerDp = a.metric.PxPerDp
	}
	x := unit.Dp(float32(sz.X) / pxPerDp)
	y := unit.Dp(float32(sz.Y) / pxPerDp)
	a.appWindow.Option(app.Size(x, y))
	log(LogCatgApp, "Application: setting window size to %v\n", sz)
}

func (a *Application) WinIdGenerator() *IdGen {
	return &a.winIdGenerator
}

func (a *Application) ColIdGenerator() *IdGen {
	return &a.colIdGenerator
}
