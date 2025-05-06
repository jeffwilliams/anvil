package main

import (
	"github.com/pkg/profile"
)

type ProfileCategory string

const (
	ProfileCPU  ProfileCategory = "CPU"
	ProfileHeap                 = "Heap"
)

var profiler interface {
	Stop()
}

func startProfiling(what ProfileCategory) {
	switch what {
	case ProfileCPU:
		profiler = profile.Start(profile.ProfilePath("."))
	case ProfileHeap:
		profiler = profile.Start(profile.MemProfileHeap, profile.ProfilePath("."))
	}
}

func isProfiling() bool {
	return profiler != nil
}

func stopProfiling() {
	if isProfiling() {
		profiler.Stop()
	}
}
