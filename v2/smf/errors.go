package smf

import "errors"

var errUnexpectedEOF = errors.New("Unexpected End of File found.")
var (
	errUnsupportedSMFFormat  = errors.New("SMF format not expected")
	ErrExpectedMIDIHeader    = errors.New("expected SMF Midi header")
	errBadSizeChunk          = errors.New("chunk was an unexpected size.")
	errInterruptedByCallback = errors.New("interrupted by callback")
)
