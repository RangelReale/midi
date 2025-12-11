package smf

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/v2/drivers"
)

type writerLogger struct {
	wr io.Writer
}

func (me *writerLogger) Printf(format string, vals ...interface{}) {
	fmt.Fprintf(me.wr, format, vals...)
}

func LogTo(wr io.Writer) Logger {
	return &writerLogger{wr}
}

// New returns a SMF file of format type 0 (single track), that becomes type 1 (multi track), if you add tracks
func New() *SMF {
	return newSMF(0)
}

// NewSMF1 returns a SMF file of format type 1 (multi track)
func NewSMF1() *SMF {
	return newSMF(1)
}

// NewSMF2 returns a SMF file of format type 2 (multi sequence)
func NewSMF2() *SMF {
	return newSMF(2)
}

func newSMF(format uint16) *SMF {
	s := &SMF{
		format: format,
	}
	s.TimeFormat = MetricTicks(960)
	return s
}

type SMF struct {
	// NoRunningStatus is an option for writing to not write running status
	NoRunningStatus      bool
	tempoChangesFinished bool
	finished             bool

	// format is the SMF file format: SMF0, SMF1 or SMF2.
	format uint16

	// numTracks is the number of tracks (0 indicates that the number is not set yet).
	numTracks uint16

	// Logger allows logging when reading or writing
	Logger Logger

	// TimeFormat is the time format (either MetricTicks or TimeCode).
	TimeFormat TimeFormat

	// Tracks contain the midi events
	Tracks []Track

	tempoChanges TempoChanges
}

func (me SMF) String() string {
	var bd strings.Builder

	bd.WriteString(fmt.Sprintf("#### SMF Format: %v TimeFormat: %v NumTracks: %v ####\n", me.format, me.TimeFormat.String(), len(me.Tracks)))

	for i, tr := range me.Tracks {
		bd.WriteString(fmt.Sprintf("## TRACK %v ##\n", i))

		for _, ev := range tr {
			bd.WriteString(fmt.Sprintf("#%v [%v] %s\n", i, ev.Delta, ev.Message.String()))
		}
	}

	return bd.String()
}

// ConvertToSMF1 converts a given SMF format 0 to SMF format 1
// channel messages are distributed over the tracks by their channels
// e.g. channel 0 -> track 1, channel 1 -> track 2 etc.
// and everything else stays in track 0
func (me SMF) ConvertToSMF1() (dest SMF) {
	if me.format == 1 {
		return me
	}

	var channelTracks [16]TrackEvents
	var metaTrack TrackEvents

	var absTicks int64
	for _, ev := range me.Tracks[0] {
		absTicks += int64(ev.Delta)
		var te TrackEvent
		te.AbsTicks = absTicks
		te.Message = ev.Message

		var channel uint8
		if ev.Message.GetChannel(&channel) {
			channelTracks[int(channel)] = append(channelTracks[int(channel)], &te)
		} else {
			metaTrack = append(metaTrack, &te)
		}
	}

	sort.Sort(metaTrack)

	var metaTarget Track

	var lastAbs int64

	for _, ev := range metaTrack {
		delta := uint32(ev.AbsTicks - lastAbs)
		metaTarget.Add(delta, ev.Message)
		lastAbs = ev.AbsTicks
	}

	dest.TimeFormat = me.TimeFormat
	dest.format = 1

	metaTarget.Close(0)
	dest.Add(metaTarget)

	for i := 0; i < 16; i++ {
		evts := channelTracks[i]
		if len(evts) > 0 {
			var t Track
			lastAbs = 0
			for _, ev := range evts {
				delta := uint32(ev.AbsTicks - lastAbs)
				t.Add(delta, ev.Message)
				lastAbs = ev.AbsTicks
			}
			t.Close(0)
			dest.Add(t)
		}
	}

	return dest
}

// RecordTo records from the given midi in port into the given filename with the given tempo.
// It returns a stop function that must be called to stop the recording. The file is then completed and saved.
func RecordTo(inport drivers.In, bpm float64, filename string) (stop func() error, err error) {
	file := New()
	_stop, _err := file.RecordFrom(inport, bpm)

	if _err != nil {
		_stop()
		return nil, _err
	}

	return func() error {
		_stop()
		return file.WriteFile(filename)
	}, nil
}

// RecordFrom records from the given midi in port into a new track.
// It returns a stop function that must be called to stop the recording.
// It is up to the user to save the SMF.
func (me *SMF) RecordFrom(inport drivers.In, bpm float64) (stop func(), err error) {
	ticks := me.TimeFormat.(MetricTicks)

	var tr Track

	_stop, _err := tr.RecordFrom(inport, ticks, bpm)

	if _err != nil {
		_stop()
		time.Sleep(time.Second)
		tr.Close(0)
		me.Add(tr)
		return nil, _err
	}

	return func() {
		_stop()
		time.Sleep(time.Second)
		tr.Close(0)
		me.Add(tr)
	}, nil
}

func (me *SMF) TempoChanges() TempoChanges {
	return me.tempoChanges
}

func (me *SMF) finishTempoChanges() {
	if me.tempoChangesFinished {
		return
	}
	sort.Sort(me.tempoChanges)
	me.calculateAbsTimes()
	me.tempoChangesFinished = true
}

func (me *SMF) calculateAbsTimes() {
	var lasttcTick, lasttcTimeMicroSec int64
	mt := me.TimeFormat.(MetricTicks)
	for _, tc := range me.tempoChanges {
		diffTicks := tc.AbsTicks - lasttcTick

		// if the tempo change is at the same tick as the last one, we just copy the time
		if diffTicks == 0 {
			tc.AbsTimeMicroSec = lasttcTimeMicroSec
			continue
		}

		prev := me.tempoChanges.TempoChangeAt(tc.AbsTicks - 1)
		var prevTime int64
		if prev != nil {
			prevTime = prev.AbsTimeMicroSec
		}
		prevTempo := me.tempoChanges.TempoAt(tc.AbsTicks - 1)
		//fmt.Printf("tc at: %v diff ticks: %v (uint32: %v)\n", tc.AbsTicks, diffTicks, uint32(diffTicks))
		// calculate time for diffTicks with the help of the last tempo and the MetricTicks
		tc.AbsTimeMicroSec = prevTime + mt.Duration(prevTempo, uint32(diffTicks)).Microseconds()

		lasttcTick = tc.AbsTicks
		lasttcTimeMicroSec = tc.AbsTimeMicroSec
	}
}

// TimeAt returns the absolute time for a given absolute tick (considering the tempo changes)
func (me *SMF) TimeAt(absTicks int64) (absTimeMicroSec int64) {
	me.finishTempoChanges()
	mt := me.TimeFormat.(MetricTicks)
	prevTc := me.tempoChanges.TempoChangeAt(absTicks - 1)
	if prevTc == nil {
		return mt.Duration(120.00, uint32(absTicks)).Microseconds()
	}
	return prevTc.AbsTimeMicroSec + mt.Duration(prevTc.BPM, uint32(absTicks-prevTc.AbsTicks)).Microseconds()
}

// NumTracks returns the number of tracks
func (me *SMF) NumTracks() uint16 {
	return uint16(len(me.Tracks))
}

// WriteFile writes the SMF to the given filename
func (me *SMF) WriteFile(file string) error {
	f, err := os.Create(file)

	if err != nil {
		return fmt.Errorf("writing midi file failed: could not create file %#v", file)
	}

	//err = s.WriteTo(f)
	_, err = me.WriteTo(f)
	f.Close()

	if err != nil {
		os.Remove(file)
		return fmt.Errorf("writing to midi file %#v failed: %v", file, err)
	}

	return nil
}

func (me *SMF) Bytes() (data []byte, err error) {
	var bf bytes.Buffer
	_, err = me.WriteTo(&bf)
	if err != nil {
		return
	}
	return bf.Bytes(), nil
}

// WriteTo writes the SMF to the given writer
func (me *SMF) WriteTo(f io.Writer) (size int64, err error) {
	me.numTracks = uint16(len(me.Tracks))
	if me.numTracks == 0 {
		return 0, fmt.Errorf("no track added")
	}
	if me.numTracks > 1 && me.format == 0 {
		me.format = 1
	}

	for i := range me.Tracks {
		if !me.Tracks[i].IsClosed() {
			if me.Logger != nil {
				me.Logger.Printf("track %v is not closed, adding end with delta 0", i)
			}
			me.Tracks[i].Close(0)
		}
	}

	//fmt.Printf("numtracks: %v\n", s.numTracks)
	wr := newWriter(me, f)
	err = wr.WriteHeader()
	if err != nil {
		return 0, fmt.Errorf("could not write header: %v", err)
	}

	for _, t := range me.Tracks {
		for _, ev := range t {
			//fmt.Printf("written ev: %v\n ", ev)
			wr.SetDelta(ev.Delta)
			err = wr.Write(ev.Message)
			if err != nil {
				break
			}
		}

		err = wr.writeChunkTo(wr.output)

		if err != nil {
			break
		}
	}

	return wr.output.size, nil
}

func (me *SMF) log(format string, vals ...interface{}) {
	if me.Logger != nil {
		me.Logger.Printf(format+"\n", vals...)
	}
}

// Add adds a track to the SMF and returns an error, if the track is not closed.
func (me *SMF) Add(t Track) error {
	if me.Logger != nil {
		me.log("add track %v", len(me.Tracks)+1)

		for _, ev := range t {
			me.log("delta: %v message: %s", ev.Delta, ev.Message)
		}
	}
	me.Tracks = append(me.Tracks, t)
	if len(me.Tracks) > 1 && me.format == 0 {
		me.format = 1
	}
	if !t.IsClosed() {
		me.log("error: track %v was not closed", len(me.Tracks))
		return fmt.Errorf("error: track %v was not closed", len(me.Tracks))
	}
	return nil
}

func (me SMF) Format() uint16 {
	return me.format
}
