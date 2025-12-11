package smf

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"time"

	"reflect"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type Event struct {
	Delta   uint32
	Message Message
}

type Track []Event

func (me Track) IsClosed() bool {
	if len(me) == 0 {
		return false
	}

	last := me[len(me)-1]
	return reflect.DeepEqual(last.Message, EOT)
}

func (me Track) IsEmpty() bool {
	if me.IsClosed() {
		return len(me) == 1
	}
	return len(me) == 0
}

func (me *Track) Close(deltaticks uint32) {
	if me.IsClosed() {
		return
	}
	*me = append(*me, Event{Delta: deltaticks, Message: EOT})
}

func (me *Track) Add(deltaticks uint32, msgs ...[]byte) {
	if me.IsClosed() {
		return
	}
	for _, msg := range msgs {
		ev := Event{Delta: deltaticks, Message: msg}
		*me = append(*me, ev)
		deltaticks = 0
	}
}

func (me *Track) RecordFrom(inPort drivers.In, ticks MetricTicks, bpm float64) (stop func(), err error) {
	if !inPort.IsOpen() {
		err := inPort.Open()
		if err != nil {
			return nil, err
		}
	}
	me.Add(0, MetaTempo(bpm))
	var absmillisec int32
	return midi.ListenTo(inPort, func(msg midi.Message, absms int32) {
		deltams := absms - absmillisec
		absmillisec = absms
		delta := ticks.Ticks(bpm, time.Duration(deltams)*time.Millisecond)
		me.Add(delta, msg)
	})
}

func (me *Track) SendTo(resolution MetricTicks, tc TempoChanges, receiver func(m midi.Message, timestampms int32)) {
	var absDelta int64

	for _, ev := range *me {
		absDelta += int64(ev.Delta)
		if Message(ev.Message).IsPlayable() {
			ms := int32(resolution.Duration(tc.TempoAt(absDelta), ev.Delta).Microseconds() * 100)
			receiver(ev.Message.Bytes(), ms)
		}
	}
}

type TracksReader struct {
	smf    *SMF
	tracks map[int]bool
	filter []midi.Type
	err    error
}

func (me *TracksReader) Error() error {
	return me.err
}

func (me *TracksReader) SMF() *SMF {
	return me.smf
}

func (me *TracksReader) doTrack(tr int) bool {
	if len(me.tracks) == 0 {
		return true
	}

	return me.tracks[tr]
}

func ReadTracks(filepath string, tracks ...int) *TracksReader {
	t := &TracksReader{}
	t.tracks = map[int]bool{}
	for _, tr := range tracks {
		t.tracks[tr] = true
	}
	t.smf, t.err = ReadFile(filepath)
	if t.err != nil {
		return t
	}
	if _, ok := t.smf.TimeFormat.(MetricTicks); !ok {
		t.err = fmt.Errorf("SMF time format is not metric ticks, but %s (currently not supported)", t.smf.TimeFormat.String())
		return t
	}
	return t
}

func ReadTracksFrom(rd io.Reader, tracks ...int) *TracksReader {
	t := &TracksReader{}
	t.tracks = map[int]bool{}
	for _, tr := range tracks {
		t.tracks[tr] = true
	}

	t.smf, t.err = ReadFrom(rd)
	if t.err != nil {
		return t
	}
	if _, ok := t.smf.TimeFormat.(MetricTicks); !ok {
		t.err = fmt.Errorf("SMF time format is not metric ticks, but %s (currently not supported)", t.smf.TimeFormat.String())
		return t
	}
	return t
}

func (me *TracksReader) Only(mtypes ...midi.Type) *TracksReader {
	me.filter = mtypes
	return me
}

type TrackEvent struct {
	Event
	TrackNo         int
	AbsTicks        int64
	AbsMicroSeconds int64
}

type TrackEvents []*TrackEvent

func (me TrackEvents) Len() int {
	return len(me)
}

func (me TrackEvents) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me TrackEvents) Less(a, b int) bool {
	return me[a].AbsTicks < me[b].AbsTicks
}

type playEvent struct {
	absTime int64
	sleep   time.Duration
	data    []byte
	out     drivers.Out
	trackNo int
	//str     string
}

type player []playEvent

func (me player) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me player) Less(a, b int) bool {
	return me[a].absTime < me[b].absTime
}

func (me player) Len() int {
	return len(me)
}

// Play plays the tracks on the given out port
func (me *TracksReader) Play(out drivers.Out) error {
	if me.err != nil {
		return me.err
	}

	err := out.Open()
	if err != nil {
		return err
	}

	return me.MultiPlay(map[int]drivers.Out{-1: out})
}

// MultiPlay plays tracks to different out ports.
// If the map has an index of -1, it will be used to play all tracks that have no explicit out port.
func (me *TracksReader) MultiPlay(trackouts map[int]drivers.Out) error {
	if me.err != nil {
		return me.err
	}
	var pl player
	if len(trackouts) == 0 {
		me.err = fmt.Errorf("trackouts not set")
		return me.err
	}

	me.Do(
		func(te TrackEvent) {
			msg := te.Message
			if msg.IsPlayable() {
				var out drivers.Out

				if o, has := trackouts[te.TrackNo]; has {
					out = o
				} else {
					if def, hasDef := trackouts[-1]; hasDef {
						out = def
					} else {
						return
					}
				}

				pl = append(pl, playEvent{
					absTime: te.AbsMicroSeconds,
					data:    msg,
					out:     out,
					trackNo: te.TrackNo,
				})
			}
		},
	)

	sort.Sort(pl)

	var last time.Duration = 0

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for i := range pl {
		last = me.play(last, pl[i])
	}

	return me.err
}

func (me *TracksReader) play(last time.Duration, p playEvent) time.Duration {
	current := (time.Microsecond * time.Duration(p.absTime))
	diff := current - last
	time.Sleep(diff)
	p.out.Send(p.data)
	return current
}

func (me *TracksReader) Do(fn func(TrackEvent)) *TracksReader {
	if me.err != nil {
		return me
	}
	tracks := me.smf.Tracks

	for no, tr := range tracks {
		if me.doTrack(no) {
			var absTicks int64
			for _, ev := range tr {
				te := TrackEvent{Event: ev, TrackNo: no}
				d := int64(ev.Delta)
				te.AbsTicks = absTicks + d
				te.AbsMicroSeconds = me.smf.TimeAt(te.AbsTicks)
				if me.filter == nil {
					fn(te)
				} else {
					msg := ev.Message
					ty := msg.Type()
					for _, f := range me.filter {
						if ty.Is(f) {
							fn(te)
						}
					}
				}
				absTicks = te.AbsTicks
			}
		}
	}

	return me
}
