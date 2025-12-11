//go:build js && wasm && !windows && !linux && !darwin
// +build js,wasm,!windows,!linux,!darwin

package webmididrv

import (
	"fmt"
	"strings"
	"sync"
	"syscall/js"

	"gitlab.com/gomidi/midi/v2/drivers"
)

func init() {
	drv, err := New()
	if err != nil {
		panic(fmt.Sprintf("could not register webmididrv: %s", err.Error()))
	}
	drivers.Register(drv)
}

type Driver struct {
	opened []drivers.Port
	sync.RWMutex
	inputsJS  js.Value
	outputsJS js.Value
	wg        sync.WaitGroup
	Err       error
}

func (me *Driver) String() string {
	return "webmididrv"
}

// Close closes all open ports. It must be called at the end of a session.
func (me *Driver) Close() (err error) {
	me.Lock()
	var e CloseErrors

	for _, p := range me.opened {
		err = p.Close()
		if err != nil {
			e = append(e, err)
		}
	}

	me.Unlock()

	if len(e) == 0 {
		return nil
	}

	return e
}

// New returns a driver based on the js webmidi standard
func New() (*Driver, error) {
	jsDoc := js.Global().Get("navigator")
	if !jsDoc.Truthy() {
		return nil, fmt.Errorf("Unable to get navigator object")
	}

	// currently sysex messages are not allowed in the browser implementations
	var opts = map[string]interface{}{
		"sysex": "false",
	}

	jsOpts := js.ValueOf(opts)

	midiaccess := jsDoc.Call("requestMIDIAccess", jsOpts)
	if !midiaccess.Truthy() {
		return nil, fmt.Errorf("unable to get requestMIDIAccess")
	}

	drv := &Driver{}
	drv.wg.Add(1)
	midiaccess.Call("then", drv.onMIDISuccess(), drv.onMIDIFailure())
	drv.wg.Wait()
	return drv, nil
}

func (me *Driver) onMIDISuccess() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return "Invalid no of arguments passed"
		}

		me.inputsJS = args[0].Get("inputs")
		me.outputsJS = args[0].Get("outputs")
		me.wg.Done()
		return nil
	})
}

func (me *Driver) onMIDIFailure() js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		me.Err = fmt.Errorf("Could not access the MIDI devices.")
		me.wg.Done()
		return nil
	})
}

// Ins returns the available MIDI input ports
func (me *Driver) Ins() (ins []drivers.In, err error) {
	if me.Err != nil {
		return nil, err
	}

	if !me.inputsJS.Truthy() {
		return nil, fmt.Errorf("no inputs")
	}

	var i = 0

	eachIn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		jsport := args[0]
		var name = jsport.Get("name").String()
		ins = append(ins, newIn(me, i, name, jsport))
		i++
		return nil
	})

	me.inputsJS.Call("forEach", eachIn)
	return ins, nil
}

// Outs returns the available MIDI output ports
func (me *Driver) Outs() (outs []drivers.Out, err error) {
	if me.Err != nil {
		return nil, err
	}

	if !me.outputsJS.Truthy() {
		return nil, fmt.Errorf("no outputs")
	}

	var i = 0

	eachOut := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		jsport := args[0]
		var name = jsport.Get("name").String()
		outs = append(outs, newOut(me, i, name, jsport))
		i++
		return nil
	})

	me.outputsJS.Call("forEach", eachOut)

	return outs, nil
}

// CloseErrors collects error from closing multiple MIDI ports
type CloseErrors []error

func (me CloseErrors) Error() string {
	if len(me) == 0 {
		return "no errors"
	}

	var bd strings.Builder

	bd.WriteString("the following closing errors occured:\n")

	for _, e := range me {
		bd.WriteString(e.Error() + "\n")
	}

	return bd.String()
}
