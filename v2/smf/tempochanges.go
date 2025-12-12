package smf

type TempoChange struct {
	AbsTicks        int64
	AbsTimeMicroSec int64
	BPM             float64
}

type TempoChanges []*TempoChange

func (me TempoChanges) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me TempoChanges) Len() int {
	return len(me)
}

func (me TempoChanges) Less(a, b int) bool {
	return me[a].AbsTicks < me[b].AbsTicks
}

func (me TempoChanges) TempoAt(absTicks int64) (bpm float64) {
	tc := me.TempoChangeAt(absTicks)
	if tc == nil {
		return 120.00
	}
	return tc.BPM
}

func (me TempoChanges) TempoChangeAt(absTicks int64) (tch *TempoChange) {
	for _, tc := range me {
		if tc.AbsTicks > absTicks {
			break
		}
		tch = tc
	}
	return
}
