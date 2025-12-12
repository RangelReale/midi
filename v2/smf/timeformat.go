package smf

import (
	"fmt"
	"math"
	"time"
)

var (
	_ TimeFormat = MetricTicks(0)
	_ TimeFormat = TimeCode{}
)

// TimeFormat is the common interface of all SMF time formats
type TimeFormat interface {
	String() string
	timeformat() // make the implementation exclusive to this package
}

// TimeCode is the SMPTE time format.
// It can be comfortable created with the SMPTE* functions.
type TimeCode struct {
	FramesPerSecond uint8
	SubFrames       uint8
}

// String represents the TimeCode as a string.
func (me TimeCode) String() string {

	switch me.FramesPerSecond {
	case 29:
		return fmt.Sprintf("SMPTE30DropFrame %v subframes", me.SubFrames)
	default:
		return fmt.Sprintf("SMPTE%v %v subframes", me.FramesPerSecond, me.SubFrames)
	}

}

func (me TimeCode) timeformat() {}

// SMPTE24 returns a SMPTE24 TimeCode with the given subframes.
func SMPTE24(subframes uint8) TimeCode {
	return TimeCode{24, subframes}
}

// SMPTE25 returns a SMPTE25 TimeCode with the given subframes.
func SMPTE25(subframes uint8) TimeCode {
	return TimeCode{25, subframes}
}

// SMPTE30DropFrame returns a SMPTE30 drop frame TimeCode with the given subframes.
func SMPTE30DropFrame(subframes uint8) TimeCode {
	return TimeCode{29, subframes}
}

// SMPTE30 returns a SMPTE30 TimeCode with the given subframes.
func SMPTE30(subframes uint8) TimeCode {
	return TimeCode{30, subframes}
}

// MetricTicks represents the "ticks per quarter note" (metric) time format.
// It defaults to 960 (i.e. 0 is treated as if it where 960 ticks per quarter note).
type MetricTicks uint16

const defaultMetric MetricTicks = 960

// In64ths returns the deltaTicks in 64th notes.
// To get 32ths, divide result by 2.
// To get 16ths, divide result by 4.
// To get 8ths, divide result by 8.
// To get 4ths, divide result by 16.
func (me MetricTicks) In64ths(deltaTicks uint32) uint32 {
	if me == 0 {
		me = defaultMetric
	}
	return (deltaTicks * 16) / uint32(me)
}

// Duration returns the time.Duration for a number of ticks at a certain tempo (in fractional BPM)
func (me MetricTicks) Duration(fractionalBPM float64, deltaTicks uint32) time.Duration {
	if me == 0 {
		me = defaultMetric
	}
	// (60000 / T) * (d / R) = D[ms]
	//	durQnMilli := 60000 / float64(tempoBPM)
	//	_4thticks := float64(deltaTicks) / float64(uint16(q))
	res := 60000000000 * float64(deltaTicks) / (fractionalBPM * float64(uint16(me)))
	//fmt.Printf("what: %vns\n", res)
	return time.Duration(int64(math.Round(res)))
	//	return time.Duration(roundFloat(durQnMilli*_4thticks, 0)) * time.Millisecond
}

// Ticks returns the ticks for a given time.Duration at a certain tempo (in fractional BPM)
func (me MetricTicks) Ticks(fractionalBPM float64, d time.Duration) (ticks uint32) {
	if me == 0 {
		me = defaultMetric
	}
	// d = (D[ms] * R * T) / 60000
	ticks = uint32(math.Round((float64(d.Nanoseconds()) / 1000000 * float64(uint16(me)) * fractionalBPM) / 60000))
	return ticks
}

func (me MetricTicks) div(d float64) uint32 {
	if me == 0 {
		me = defaultMetric
	}
	return uint32(math.Round(float64(me.Resolution()) / d))
}

// Resolution returns the number of the metric ticks (ticks for a quarter note, defaults to 960)
func (me MetricTicks) Resolution() uint16 {
	if me == 0 {
		me = defaultMetric
	}
	return uint16(me)
}

// Ticks4th returns the ticks for a quarter note
func (me MetricTicks) Ticks4th() uint32 {
	return uint32(me.Resolution())
}

// Ticks8th returns the ticks for a quaver note
func (me MetricTicks) Ticks8th() uint32 {
	return me.div(2)
}

// Ticks16th returns the ticks for a 16th note
func (me MetricTicks) Ticks16th() uint32 {
	return me.div(4)
}

// Ticks32th returns the ticks for a 32th note
func (me MetricTicks) Ticks32th() uint32 {
	return me.div(8)
}

// Ticks64th returns the ticks for a 64th note
func (me MetricTicks) Ticks64th() uint32 {
	return me.div(16)
}

// Ticks128th returns the ticks for a 128th note
func (me MetricTicks) Ticks128th() uint32 {
	return me.div(32)
}

// Ticks256th returns the ticks for a 256th note
func (me MetricTicks) Ticks256th() uint32 {
	return me.div(64)
}

// Ticks512th returns the ticks for a 512th note
func (me MetricTicks) Ticks512th() uint32 {
	return me.div(128)
}

// Ticks1024th returns the ticks for a 1024th note
func (me MetricTicks) Ticks1024th() uint32 {
	return me.div(256)
}

// String returns the string representation of the quarter note resolution
func (me MetricTicks) String() string {
	return fmt.Sprintf("%v MetricTicks", me.Resolution())
}

func (me MetricTicks) timeformat() {}
