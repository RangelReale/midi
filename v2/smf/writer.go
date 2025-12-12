package smf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"gitlab.com/gomidi/midi/v2/internal/runningstatus"
	vlq "gitlab.com/gomidi/midi/v2/internal/utils"
)

type wrWrapper struct {
	size int64
	wr   io.Writer
}

var _ io.Writer = &wrWrapper{}

func (me *wrWrapper) Write(p []byte) (int, error) {
	s, err := me.wr.Write(p)
	me.size += int64(s)
	return s, err
}

func newWriter(s *SMF, output io.Writer) *writer {
	// setup
	me := &writer{}
	me.SMF = s
	me.output = &wrWrapper{wr: output}
	me.currentChunk.SetType([4]byte{byte('M'), byte('T'), byte('r'), byte('k')})

	if !me.SMF.NoRunningStatus {
		me.runningWriter = runningstatus.NewSMFWriter()
	}
	return me
}

type writer struct {
	*SMF
	currentChunk    chunk
	output          *wrWrapper
	headerWritten   bool
	tracksProcessed uint16
	deltatime       uint32
	absPos          uint64
	error           error
	runningWriter   runningstatus.SMFWriter
}

func (me *writer) printf(format string, vals ...interface{}) {
	if me.SMF.Logger == nil {
		return
	}

	me.SMF.Logger.Printf("smfwriter: "+format+"\n", vals...)
}

func (me *writer) Close() error {
	if cl, is := me.output.wr.(io.WriteCloser); is {
		me.printf("closing output")
		return cl.Close()
	}
	return nil
}

func (me *writer) WriteHeader() error {
	if me.headerWritten {
		return me.error
	}
	err := me.writeHeader(me.output)
	me.headerWritten = true

	if err != nil {
		me.error = err
	}

	return err
}

func (me *writer) Position() uint64 {
	return me.absPos
}

// SetDelta sets the delta time in ticks for the next message(s)
func (me *writer) SetDelta(deltatime uint32) {
	me.deltatime = deltatime
}

// Write writes the message and returns the bytes that have been physically written.
// If a write fails with an error, every following attempt to write will return this first error,
// so de facto writing will be blocked.
func (me *writer) Write(m Message) (err error) {
	if me.error != nil {
		return me.error
	}
	if !me.headerWritten {
		me.error = me.WriteHeader()
	}
	if me.error != nil {
		me.printf("ERROR: writing header before midi message %#v failed: %v", m, me.error)
		me.error = fmt.Errorf("writing header before midi message %#v failed: %v", m, me.error)
		return me.error
	}
	defer func() {
		me.deltatime = 0
	}()

	me.addMessage(me.deltatime, m)
	return
}

/*
				| time type            | bit 15 | bits 14 thru 8        | bits 7 thru 0   |
				-----------------------------------------------------------------------------
			  | metrical time        |      0 |         ticks per quarter-note          |
			  | time-code-based time |      1 | negative SMPTE format | ticks per frame |

	If bit 15 of <division> is zero, the bits 14 thru 0 represent the number of delta time "ticks" which make up a
	quarter-note. For instance, if division is 96, then a time interval of an eighth-note between two events in the
	file would be 48.

	If bit 15 of <division> is a one, delta times in a file correspond to subdivisions of a second, in a way
	consistent with SMPTE and MIDI Time Code. Bits 14 thru 8 contain one of the four values -24, -25, -29, or
	-30, corresponding to the four standard SMPTE and MIDI Time Code formats (-29 corresponds to 30 drop
	frame), and represents the number of frames per second. These negative numbers are stored in two's
	compliment form. The second byte (stored positive) is the resolution within a frame: typical values may be 4
	(MIDI Time Code resolution), 8, 10, 80 (bit resolution), or 100. This stream allows exact specifications of
	time-code-based tracks, but also allows millisecond-based tracks by specifying 25 frames/sec and a resolution
	of 40 units per frame. If the events in a file are stored with a bit resolution of thirty-frame time code, the
	division word would be E250 hex. (=> 1110001001010000 or 57936)

/* unit of time for delta timing. If the value is positive, then it represents the units per beat.
For example, +96 would mean 96 ticks per beat. If the value is negative, delta times are in SMPTE compatible units.
*/
func (me *writer) writeTimeFormat(wr io.Writer) error {
	switch tf := me.SMF.TimeFormat.(type) {
	case MetricTicks:
		ticks := tf.Ticks4th()
		if ticks > 32767 {
			ticks = 32767 // 32767 is the largest possible value, since bit 15 must always be 0
		}
		me.printf("writing metric ticks: %v", ticks)
		return binary.Write(wr, binary.BigEndian, uint16(ticks))
	case TimeCode:
		// multiplication with -1 makes sure that bit 15 is set
		err := binary.Write(wr, binary.BigEndian, int8(tf.FramesPerSecond)*-1)
		if err != nil {
			return err
		}
		me.printf("writing time code fps: %v subframes: %v", int8(tf.FramesPerSecond)*-1, tf.SubFrames)
		return binary.Write(wr, binary.BigEndian, tf.SubFrames)
	default:
		//panic(fmt.Sprintf("unsupported TimeFormat: %#v", w.header.TimeFormat))
		me.printf("ERROR: unsupported TimeFormat: %#v", me.SMF.TimeFormat)
		return fmt.Errorf("unsupported TimeFormat: %#v", me.SMF.TimeFormat)
	}
}

// <Header Chunk> = <chunk type><length><format><ntrks><division>
func (me *writer) writeHeader(wr io.Writer) error {
	me.printf("write header")
	var ch chunk
	ch.SetType([4]byte{byte('M'), byte('T'), byte('h'), byte('d')})
	var bf bytes.Buffer

	me.printf("write format %v", me.format)
	binary.Write(&bf, binary.BigEndian, me.format)
	me.printf("write num tracks %v", me.numTracks)
	binary.Write(&bf, binary.BigEndian, me.numTracks)

	err := me.writeTimeFormat(&bf)
	if err != nil {
		me.printf("ERROR: could not write header: %v", err)
		return fmt.Errorf("could not write header: %v", err)
	}

	_, err = ch.Write(bf.Bytes())
	if err != nil {
		me.printf("ERROR: could not write header: %v", err)
		return fmt.Errorf("could not write header: %v", err)
	}

	_, err = ch.WriteTo(wr)
	if err != nil {
		me.printf("ERROR: could not write header: %v", err)
		return fmt.Errorf("could not write header: %v", err)
	}
	me.printf("header written successfully")
	return nil
}

// <Track Chunk> = <chunk type><length><MTrk event>+
func (me *writer) writeChunkTo(wr io.Writer) (err error) {
	_, err = me.currentChunk.WriteTo(wr)

	if err != nil {
		me.printf("ERROR: could not write track %v: %v", me.tracksProcessed+1, err)
		return fmt.Errorf("could not write track %v: %v", me.tracksProcessed+1, err)
	}

	me.printf("track %v successfully written", me.tracksProcessed+1)

	if !me.SMF.NoRunningStatus {
		me.runningWriter = runningstatus.NewSMFWriter()
	}

	// remove the data for the next track
	me.currentChunk.Clear()
	me.deltatime = 0

	me.tracksProcessed++
	if me.numTracks == me.tracksProcessed {
		me.printf("last track written, finished")
		//		err = ErrFinished
	}

	return
}

func (me *writer) appendToChunk(deltaTime uint32, b []byte) {
	me.currentChunk.Write(append(vlq.VlqEncode(deltaTime), b...))
}

// delta is distance in time to last event in this track (independent of the channel)
func (me *writer) addMessage(deltaTime uint32, raw Message) {
	me.absPos += uint64(deltaTime)

	isSysEx := raw[0] == 0xF0 || raw[0] == 0xF7
	if isSysEx {
		// we have some sort of sysex, so we need to
		// calculate the length of msg[1:]
		// set msg to msg[0] + length of msg[1:] + msg[1:]
		if me.runningWriter != nil {
			me.runningWriter.ResetStatus()
		}

		//if sys, ok := msg.(sysex.Message); ok {
		b := []byte{raw[0]}
		b = append(b, vlq.VlqEncode(uint32(len(raw)-1))...)
		if len(raw[1:]) != 0 {
			b = append(b, raw[1:]...)
		}

		me.appendToChunk(deltaTime, b)
		return
	}

	if me.runningWriter != nil {
		me.appendToChunk(deltaTime, me.runningWriter.Write(raw))
		return
	}

	me.appendToChunk(deltaTime, raw)
}

/*
from http://www.artandscienceofsound.com/article/standardmidifiles

Depending upon the application you are using to create the file in the first place, header information may automatically be saved from within parameters set in the application, or may need to be placed in a ‘set-up’ bar before the music data commences.

Either way, information that should be considered includes:

GM/GS Reset message

Per MIDI Channel
Bank Select (0=GM) / Program Change #
Reset All Controllers (not all devices may recognize this command so you may prefer to zero out or reset individual controllers)
Initial Volume (CC7) (standard level = 100)
Expression (CC11) (initial level set to 127)
Hold pedal (0 = off)
Pan (Center = 64)
Modulation (0)
Pitch bend range
Reverb (0 = off)
Chorus level (0 = off)

System Exclusive data

If RPNs or more detailed controller messages are being employed in the file these should also be reset or normalized in the header.

If you are inputting header data yourself it is advisable not to clump all such information together but rather space it out in intervals of 5-10 ticks. Certainly if a file is designed to be looped, having too much data play simultaneously will cause most playback devices to ‘choke, ’ and throw off your timing.
*/

/*
from http://www.artandscienceofsound.com/article/standardmidifiles

Depending upon the application you are using to create the file in the first place, header information may automatically be saved from within parameters set in the application, or may need to be placed in a ‘set-up’ bar before the music data commences.

Either way, information that should be considered includes:

GM/GS Reset message

Per MIDI Channel
Bank Select (0=GM) / Program Change #
Reset All Controllers (not all devices may recognize this command so you may prefer to zero out or reset individual controllers)
Initial Volume (CC7) (standard level = 100)
Expression (CC11) (initial level set to 127)
Hold pedal (0 = off)
Pan (Center = 64)
Modulation (0)
Pitch bend range
Reverb (0 = off)
Chorus level (0 = off)

System Exclusive data

If RPNs or more detailed controller messages are being employed in the file these should also be reset or normalized in the header.

If you are inputting header data yourself it is advisable not to clump all such information together but rather space it out in intervals of 5-10 ticks. Certainly if a file is designed to be looped, having too much data play simultaneously will cause most playback devices to ‘choke, ’ and throw off your timing.
*/
