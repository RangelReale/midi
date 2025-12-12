//go:build !js

package midicatdrv

import (
	"fmt"
	"io"
	"os/exec"
	"sync"

	"gitlab.com/gomidi/midi/v2/drivers"
)

func newOut(driver *Driver, number int, name string) drivers.Out {
	o := &out{driver: driver, number: number, name: name}
	return o
}

type out struct {
	number int
	sync.RWMutex
	driver *Driver
	name   string
	wr     *io.PipeWriter
	rd     *io.PipeReader
	cmd    *exec.Cmd
}

func (me *out) fireCmd() error {
	me.Lock()
	defer me.Unlock()
	if me.cmd != nil {
		return fmt.Errorf("already running")
	}

	me.cmd = midiCatOutCmd(me.number)
	me.rd, me.wr = io.Pipe()
	me.cmd.Stdin = me.rd

	err := me.cmd.Start()
	if err != nil {
		me.rd = nil
		me.wr = nil
		me.cmd = nil
		return err
	}

	return err
}

// IsOpen returns wether the port is open
func (me *out) IsOpen() (open bool) {
	me.RLock()
	open = me.cmd != nil
	me.RUnlock()
	return
}

// Send sends a MIDI message to the MIDI output port
// If the output port is closed, it returns midi.ErrClosed
func (me *out) Send(b []byte) error {
	me.Lock()
	defer me.Unlock()
	if me.cmd == nil {
		fmt.Println("port closed")
		return drivers.ErrPortClosed
	}
	//fmt.Printf("% X\n", b)
	_, err := fmt.Fprintf(me.wr, "%d %X\n", 0, b)
	//_, err := fmt.Fprintf(o.wr, "%X\n", b)
	if err != nil {
		return err
	}
	return nil
}

// Underlying returns the underlying driver. Here it returns nil
func (me *out) Underlying() interface{} {
	return nil
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
	me.wr.Close()
	err = me.cmd.Process.Kill()
	me.cmd = nil
	me.rd.Close()
	me.wr = nil
	me.rd = nil
	return err
}

// Open opens the MIDI out port
func (me *out) Open() (err error) {
	if me.IsOpen() {
		return nil
	}

	err = me.fireCmd()

	if err != nil {
		return fmt.Errorf("can't open MIDI out port %v (%s): %v", me.number, me, err)
	}

	me.driver.Lock()
	me.driver.opened = append(me.driver.opened, me)
	me.driver.Unlock()

	return nil
}
