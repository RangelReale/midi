//go:build js && wasm && !windows && !linux && !darwin
// +build js,wasm,!windows,!linux,!darwin

package webmididrv

import (
	"bytes"
	"sync"
	"syscall/js"

	"gitlab.com/gomidi/midi/v2/drivers"
)

func newOut(driver *Driver, number int, name string, jsport js.Value) drivers.Out {
	o := &out{driver: driver, number: number, name: name, jsport: jsport}
	return o
}

type out struct {
	number int
	sync.RWMutex
	driver  *Driver
	name    string
	jsport  js.Value
	isOpen  bool
	bf      bytes.Buffer
	running *drivers.Reader
}

// IsOpen returns wether the port is open
func (me *out) IsOpen() (open bool) {
	me.RLock()
	open = me.isOpen
	me.RUnlock()
	return
}

// Send writes a MIDI message to the MIDI output port
// If the output port is closed, it returns midi.ErrClosed
func (me *out) Send(b []byte) error {
	me.RLock()
	if !me.isOpen {
		me.RUnlock()
		return drivers.ErrPortClosed
	}
	me.RUnlock()

	me.running.EachMessage(b, 0)
	b = me.bf.Bytes()
	me.bf.Reset()

	var arr = make([]interface{}, len(b))
	for i, bt := range b {
		arr[i] = bt
	}

	me.jsport.Call("send", js.ValueOf(arr))
	return nil
}

// Underlying returns the underlying driver. Here it returns the js output port.
func (me *out) Underlying() interface{} {
	return me.jsport
}

// Number returns the number of the MIDI out port.
// Note that with rtmidi, out and in ports are counted separately.
// That means there might exists out ports and an in ports that share the same number
func (me *out) Number() int {
	return me.number
}

// String returns the name of the MIDI out port.
func (me *out) String() string {
	return me.name
}

// Close closes the MIDI out port
func (me *out) Close() (err error) {
	if !me.IsOpen() {
		return nil
	}

	me.Lock()
	defer me.Unlock()
	me.isOpen = false
	me.jsport.Call("close")
	return err
}

// Open opens the MIDI out port
func (me *out) Open() (err error) {
	if me.IsOpen() {
		return nil
	}

	me.driver.Lock()
	me.bf = bytes.Buffer{}
	//o.running = runningstatus.NewLiveWriter(&o.bf)
	var conf drivers.ListenConfig
	conf.ActiveSense = true
	conf.SysEx = false
	conf.TimeCode = true
	me.running = drivers.NewReader(conf, func(b []byte, ms int32) {
		me.bf.Write(b)
	})
	me.isOpen = true
	me.jsport.Call("open")
	me.driver.opened = append(me.driver.opened, me)
	me.driver.Unlock()
	return nil
}
