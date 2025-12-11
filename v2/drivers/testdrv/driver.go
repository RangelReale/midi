// Copyright (c) 2018 Marc René Arns. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

/*
Package testdrv provides a Driver for testing.
*/
package testdrv

import (
	//"sync"

	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

func init() {
	drv := New("testdrv")
	drivers.Register(drv)
}

type Driver struct {
	in            *in
	out           *out
	name          string
	last          time.Time
	now           time.Time
	stopListening bool
	rd            *drivers.Reader
	//wg            sync.WaitGroup
}

func New(name string) *Driver {
	d := &Driver{name: name}
	d.in = &in{name: name + "-in", Driver: d, number: 0}
	d.out = &out{name: name + "-out", Driver: d, number: 0}
	d.last = time.Now()
	d.now = d.last
	return d
}

func (me *Driver) Sleep(d time.Duration) {
	me.now = me.now.Add(d)
}

// wait until all messages are handled
/*
func (f *Driver) Wait() {
	f.wg.Wait()
}
*/

func (me *Driver) String() string               { return me.name }
func (me *Driver) Close() error                 { return nil }
func (me *Driver) Ins() ([]drivers.In, error)   { return []drivers.In{me.in}, nil }
func (me *Driver) Outs() ([]drivers.Out, error) { return []drivers.Out{me.out}, nil }

type in struct {
	number int
	name   string
	isOpen bool
	*Driver
}

func (me *in) String() string          { return me.name }
func (me *in) Number() int             { return me.number }
func (me *in) IsOpen() bool            { return me.isOpen }
func (me *in) Underlying() interface{} { return nil }

func (me *in) Listen(onMsg func(msg []byte, milliseconds int32), conf drivers.ListenConfig) (stopFn func(), err error) {
	//fmt.Printf("listeining from in port of %s\n", f.Driver.name)

	me.last = time.Now()

	stopFn = func() {
		me.stopListening = true
	}

	me.rd = drivers.NewReader(conf, func(m []byte, ms int32) {
		msg := midi.Message(m)

		if msg.Is(midi.ActiveSenseMsg) && !conf.ActiveSense {
			return
		}

		if msg.Is(midi.TimingClockMsg) && !conf.TimeCode {
			return
		}

		if msg.Is(midi.SysExMsg) && !conf.SysEx {
			return
		}

		//fmt.Printf("handle message % X at [%v] in driver %q\n", m, ms, f.Driver.name)
		onMsg(m, ms)
		//	f.wg.Done()
		//fmt.Println("msg handled")
	})
	me.rd.Reset()
	return stopFn, nil
}

func (me *in) Close() error {
	if !me.isOpen {
		return nil
	}
	me.isOpen = false
	return nil
}

func (me *in) Open() error {
	if me.isOpen {
		return nil
	}
	me.isOpen = true
	return nil
}

type out struct {
	number int
	name   string
	isOpen bool
	*Driver
}

func (me *out) Number() int             { return me.number }
func (me *out) IsOpen() bool            { return me.isOpen }
func (me *out) String() string          { return me.name }
func (me *out) Underlying() interface{} { return nil }

func (me *out) Close() error {
	if !me.isOpen {
		return nil
	}
	me.isOpen = false
	return nil
}

func (me *out) Send(bt []byte) error {
	if !me.isOpen {
		return drivers.ErrPortClosed
	}

	if me.stopListening {
		return nil
	}

	dur := me.now.Sub(me.last)
	ts_ms := int32(dur.Milliseconds())
	me.last = me.now
	//f.wg.Add(1)
	//fmt.Printf("message added % X (len %v) at [%v] in driver %q\n", bt, len(bt), ts_ms, f.Driver.name)
	me.rd.EachMessage(bt, ts_ms)
	/*
		f.rd.SetDelta(ts_ms)
		for _, b := range bt {
			f.rd.EachByte(b)
		}
	*/
	return nil
}

func (me *out) Open() error {
	if me.isOpen {
		return nil
	}
	me.isOpen = true
	return nil
}
