// Package safe provides panic-safe goroutine helpers. A panic in a background
// goroutine is not recovered by net/http and crashes the whole process, which
// for a single-binary server means every connected user goes down. Long-running
// background workers (schedulers, dispatchers, syncers, harvesters) should be
// launched or guarded with these helpers.
package safe

import (
	"log/slog"
	"runtime/debug"
)

// Recover is intended to be deferred at the top of a background goroutine. It
// turns a panic into a logged error instead of a process crash.
//
//	go func() {
//	    defer safe.Recover("scheduler")
//	    ...
//	}()
func Recover(name string) {
	if r := recover(); r != nil {
		slog.Error("Recovered from panic in background goroutine",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}

// Go launches fn in a panic-safe goroutine.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}
