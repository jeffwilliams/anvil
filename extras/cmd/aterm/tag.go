package main

import (
	"fmt"
	"strings"
)

func adjustTagRestart(cmdRunning bool) {
	tag, err := anvilHttpApi.WindowTag(win)
	printError(err, "getting tag failed")
	if err != nil {
		return
	}

	if cmdRunning && (strings.Contains(tag, " Restart ") || strings.HasSuffix(tag, " Restart")) {
		before, after, found := strings.Cut(tag, " Restart")
		if found {
			tag = before + after
		}
	} else if !cmdRunning && !strings.Contains(tag, " Restart ") && !strings.HasSuffix(tag, " Restart") {
		before, after, found := strings.Cut(tag, "| ")
		if found {
			tag = before + "| Restart " + after
		}
	}

	anvilHttpApi.SetWindowTag(win, tag)
}

func adjustTagAttach(attached bool) {
	tag, err := anvilHttpApi.WindowTag(win)
	printError(err, "getting tag failed")
	if err != nil {
		return
	}

	if attached {
		if strings.Contains(tag, " Attach ") || strings.HasSuffix(tag, " Attach") {
			before, after, found := strings.Cut(tag, " Attach")
			fmt.Printf("before: %s, after %s, found %v\n", before, after, found)
			if found {
				tag = before + after
			}
		}
		if !strings.Contains(tag, " Detach ") || strings.HasSuffix(tag, " Detach") {
			before, after, found := strings.Cut(tag, "| ")
			if found {
				tag = before + "| Detach " + after
			}
		}
	} else if !attached {
		if strings.Contains(tag, " Detach ") || strings.HasSuffix(tag, " Detach") {
			before, after, found := strings.Cut(tag, " Detach")
			fmt.Printf("before: %s, after %s, found %v\n", before, after, found)
			if found {
				tag = before + after
			}
		}
		if !strings.Contains(tag, " Attach ") || strings.HasSuffix(tag, " Attach") {
			before, after, found := strings.Cut(tag, "| ")
			if found {
				tag = before + "| Attach " + after
			}
		}
	}

	anvilHttpApi.SetWindowTag(win, tag)
}
