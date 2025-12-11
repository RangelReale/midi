package midicatdrv

import (
	"gitlab.com/gomidi/midi/v2/drivers"
)

type inPorts []drivers.In

func (me inPorts) Len() int {
	return len(me)
}

func (me inPorts) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me inPorts) Less(a, b int) bool {
	return me[a].Number() < me[b].Number()
}

type outPorts []drivers.Out

func (me outPorts) Len() int {
	return len(me)
}

func (me outPorts) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

func (me outPorts) Less(a, b int) bool {
	return me[a].Number() < me[b].Number()
}
