//go:build !js

package midicatdrv

import (
	"fmt"
	"io"
	"runtime"
	"sync"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
	lib "gitlab.com/gomidi/midi/v2/drivers/midicat"
)

type in struct {
	number int
	sync.RWMutex
	driver              *Driver
	name                string
	shouldStopListening chan bool
	didStopListening    chan bool
	shouldKill          chan bool
	wasKilled           chan bool
	hasProc             bool
	listener            func(data []byte, deltamillisecs int32)
}

func (me *in) fireCmd() error {
	me.Lock()
	if me.hasProc {
		me.Unlock()
		return fmt.Errorf("already running")
	}
	me.shouldStopListening = make(chan bool, 1)
	me.didStopListening = make(chan bool, 1)
	me.shouldKill = make(chan bool, 1)
	me.wasKilled = make(chan bool, 1)
	me.hasProc = true
	cmd := midiCatInCmd(me.number)
	rd, wr := io.Pipe()
	cmd.Stdout = wr
	err := cmd.Start()
	if err != nil {
		me.Lock()
		me.hasProc = false
		me.Unlock()
		return err
	}
	me.Unlock()
	go func() {
		for {
			data, abstime, err := lib.ReadAndConvert(rd)
			if err != nil {
				return
			}
			me.RLock()
			if !me.hasProc {
				me.RUnlock()
				return
			}

			if me.listener != nil {
				me.listener(data, abstime)
			}
			me.RUnlock()
			runtime.Gosched()
		}
	}()

	go func(shouldStopListening <-chan bool, didStopListening chan<- bool, shouldKill <-chan bool, wasKilled chan<- bool) {
		defer rd.Close()
		defer wr.Close()

		for {
			select {
			case <-shouldKill:
				if cmd.Process != nil {
					/*
						                                        rd.Close()
											wr.Close()
					*/
					cmd.Process.Kill()
				}
				me.Lock()
				me.hasProc = false
				me.Unlock()
				wasKilled <- true
				return
			case <-shouldStopListening:
				me.Lock()
				me.listener = nil
				me.Unlock()
				didStopListening <- true
			default:
				runtime.Gosched()
			}
		}
	}(me.shouldStopListening, me.didStopListening, me.shouldKill, me.wasKilled)

	return nil
}

// IsOpen returns wether the MIDI in port is open
func (me *in) IsOpen() (open bool) {
	me.RLock()
	open = me.hasProc
	me.RUnlock()
	return
}

// String returns the name of the MIDI in port.
func (me *in) String() string {
	return me.name
}

// Underlying returns the underlying driver. Here returns nil.
func (me *in) Underlying() interface{} {
	return nil
}

// Number returns the number of the MIDI in port.
// Note that with rtmidi, out and in ports are counted separately.
// That means there might exists out ports and an in ports that share the same number.
func (me *in) Number() int {
	return me.number
}

// Close closes the MIDI in port, after it has stopped listening.
func (me *in) Close() (err error) {
	if !me.IsOpen() {
		return nil
	}

	//i.shouldStopReading
	go func() {
		me.shouldStopListening <- true
	}()
	<-me.didStopListening

	me.shouldKill <- true
	<-me.wasKilled
	return
}

// Open opens the MIDI in port
func (me *in) Open() (err error) {
	if me.IsOpen() {
		return nil
	}

	err = me.fireCmd()
	if err != nil {
		me.Close()
		return fmt.Errorf("can't open MIDI in port %v (%s): %v", me.number, me, err)
	}

	me.driver.Lock()
	me.driver.opened = append(me.driver.opened, me)
	me.driver.Unlock()

	return nil
}

func newIn(driver *Driver, number int, name string) drivers.In {
	return &in{driver: driver, number: number, name: name}
}

func (me *in) Listen(onMsg func(msg []byte, absmilliseconds int32), conf drivers.ListenConfig) (stopFn func(), err error) {
	stopFn = func() {
		if !me.IsOpen() {
			return
		}
		me.shouldStopListening <- true
		<-me.didStopListening
	}

	if !me.IsOpen() {
		return nil, drivers.ErrPortClosed
	}

	if onMsg == nil {
		return nil, fmt.Errorf("onMsg callback must not be nil")
	}

	me.RLock()
	if me.listener != nil {
		me.RUnlock()
		return nil, fmt.Errorf("listener already set")
	}
	me.RUnlock()

	//var rd = drivers.NewReader(config, onMsg)
	me.Lock()
	me.listener = func(data []byte, absmilliseconds int32) {
		//rd.EachMessage(data, deltamillisecs)
		//rd.EachMessage(data, -1)
		msg := midi.Message(data)

		if msg.Is(midi.ActiveSenseMsg) && !conf.ActiveSense {
			return
		}

		if msg.Is(midi.TimingClockMsg) && !conf.TimeCode {
			return
		}

		if msg.Is(midi.SysExMsg) && !conf.SysEx {
			return
		}
		onMsg(data, absmilliseconds)
	}
	me.Unlock()

	return stopFn, nil
}

/*
// SendTo makes the listener listen to the in port
func (i *in) StartListening(cb func([]byte, int32)) (err error) {
	if !i.IsOpen() {
		return drivers.ErrPortClosed
	}

	i.RLock()
	if i.listener != nil {
		i.RUnlock()
		return fmt.Errorf("listener already set")
	}
	i.RUnlock()
	i.Lock()
	i.listener = cb
	i.Unlock()

	return nil
}

// StopListening cancels the listening
func (i *in) StopListening() (err error) {
	if !i.IsOpen() {
		return drivers.ErrPortClosed
	}

	i.shouldStopListening <- true
	<-i.didStopListening
	return
}
*/
