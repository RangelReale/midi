//go:build js && wasm && !windows && !linux && !darwin
// +build js,wasm,!windows,!linux,!darwin

package webmididrv

import (
	"syscall/js"

	"gitlab.com/gomidi/midi/v2/drivers"
)

func log(s string) {
	jsConsole := js.Global().Get("console")

	if !jsConsole.Truthy() {
		return
	}

	jsConsole.Call("log", js.ValueOf(s))
}

type inPorts []drivers.In

func (me inPorts) Len() int {
	return len(me)
}

func (me inPorts) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me inPorts) Less(a, b int) bool {
	return me[a].Number() < me[b].Number()
}

type outPorts []drivers.Out

func (me outPorts) Len() int {
	return len(me)
}

func (me outPorts) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me outPorts) Less(a, b int) bool {
	return me[a].Number() < me[b].Number()
}
