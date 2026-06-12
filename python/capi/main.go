//go:build golemcapi

// Command golemcapi builds golem's C-shared library (libgolem) exposing the
// string-only JSON boundary used by the Python cffi binding. It is NOT part of
// the importable `golem` package and is excluded from normal builds/tests by
// the `golemcapi` build tag; the wheel build compiles it with:
//
//	go build -tags golemcapi -buildmode=c-shared -o python/golem/libgolem.<ext> ./python/capi
//
// The Go *Engine never crosses the C boundary: engines live in a handle
// registry keyed by int64, and only JSON strings (and opaque handles) cross.
// This sidesteps every cgo/marshalling hazard — the boundary is just strings.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/enchanter-ai/golem"
)

var (
	mu      sync.Mutex
	engines = make(map[int64]*golem.Engine)
	nextID  int64
)

// GolemNewEngine constructs an engine from a JSON options string (the same
// wire-form as golem.NewEngineJSON; "" yields the default LOUD engine) and
// returns an opaque handle. A malformed options string never fails here — it is
// surfaced as a LOUD failure envelope on the first GolemEvalJSON call.
//
//export GolemNewEngine
func GolemNewEngine(optionsJSON *C.char) C.longlong {
	e := golem.NewEngineJSON(C.GoString(optionsJSON))
	mu.Lock()
	nextID++
	id := nextID
	engines[id] = e
	mu.Unlock()
	return C.longlong(id)
}

// GolemEvalJSON evaluates src against varsJSON on the engine identified by
// handle and returns a newly C.malloc'd JSON envelope string. The caller MUST
// release it with GolemFreeString. An unknown/freed handle yields a failure
// envelope rather than crashing.
//
//export GolemEvalJSON
func GolemEvalJSON(handle C.longlong, src *C.char, varsJSON *C.char) *C.char {
	mu.Lock()
	e := engines[int64(handle)]
	mu.Unlock()
	if e == nil {
		return C.CString(`{"ok":false,"errtype":"EvalError","error":"golem: invalid or freed engine handle"}`)
	}
	return C.CString(e.EvalJSON(C.GoString(src), C.GoString(varsJSON)))
}

// GolemFreeEngine drops the engine handle (and its compile cache). Safe to call
// on an unknown handle.
//
//export GolemFreeEngine
func GolemFreeEngine(handle C.longlong) {
	mu.Lock()
	delete(engines, int64(handle))
	mu.Unlock()
}

// GolemFreeString releases a string returned by GolemEvalJSON.
//
//export GolemFreeString
func GolemFreeString(s *C.char) {
	C.free(unsafe.Pointer(s))
}

func main() {}
