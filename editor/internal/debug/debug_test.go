package debug

import (
	"strings"
	"testing"
	"time"
)

func TestAll(t *testing.T) {
	l := New(10)
	l.Add("events", "Mouse clicked")
	l.Add("render", "Rendered screen")
	l.Add("events", "Something else")
	l.Add("render", "Done rendering screen")

	s := l.String()

	expected := `2022-11-10T06:53:54.600 <events><first> Mouse clicked
2022-11-10T06:53:54.600 <render><first> Rendered screen
2022-11-10T06:53:54.600 <events> Something else
2022-11-10T06:53:54.600 <render> Done rendering screen
`

	if !sameIgnoringTime(expected, s) {
		t.Fatalf("bad log:\n%s\n", s)
	}
}

func TestCatg(t *testing.T) {
	l := New(10)
	l.Add("events", "Mouse clicked")
	l.Add("render", "Rendered screen")
	l.Add("window", "Opened new window")
	l.Add("events", "Something else")
	l.Add("render", "Done rendering screen")

	s := l.String("window")

	expected := `2022-11-10T06:58:11.757 <window><first> Opened new window
`

	if !sameIgnoringTime(expected, s) {
		t.Fatalf("bad log:\n%s\n", s)
	}
}

func TestTwoCatg(t *testing.T) {
	l := New(10)
	l.Add("events", "Mouse clicked")
	l.Add("render", "Rendered screen")
	l.Add("window", "Opened new window")
	l.Add("events", "Something else")
	l.Add("render", "Done rendering screen")

	s := l.String("window", "render")

	expected := `2022-11-10T06:59:08.879 <render><first> Rendered screen
2022-11-10T06:59:08.879 <window><first> Opened new window
2022-11-10T06:59:08.879 <render> Done rendering screen
`

	if !sameIgnoringTime(expected, s) {
		t.Fatalf("bad log:\n%s\n", s)
	}
}

func TestMax(t *testing.T) {
	l := New(2)
	l.Add("events", "Mouse clicked")
	l.Add("render", "Rendered screen")
	l.Add("window", "Opened new window")
	l.Add("events", "Something else")
	l.Add("render", "Done rendering screen")
	l.Add("render", "Render 2")
	l.Add("render", "Render 2 done")
	l.Add("events", "Last Event")

	s := l.String("window", "render")

	expected := `2023-04-19T08:25:28.488 <window><first> Opened new window
2023-04-19T08:25:28.494 <render><first> Render 2
2022-11-10T06:59:08.879 <render> Render 2 done
`

	if !sameIgnoringTime(expected, s) {
		t.Fatalf("bad log:\n%s\nexpected:\n%s\n", s, expected)
	}
}

func addAndDelay(l DebugLog, category, message string) {
	l.Add(category, message)
	time.Sleep(1 * time.Millisecond)
}

func sameIgnoringTime(log1, log2 string) bool {
	lines1, lines2 := strings.Split(log1, "\n"), strings.Split(log2, "\n")

	if len(lines1) != len(lines2) {
		return false
	}

	for i := range lines1 {
		e1, e2 := withoutTime(lines1[i]), withoutTime(lines2[i])
		if e1 != e2 {
			return false
		}
	}
	return true
}

func withoutTime(logLine string) string {
	if len(logLine) < 23 {
		return logLine
	}

	return logLine[23:]
}
