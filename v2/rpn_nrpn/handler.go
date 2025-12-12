package rpn_nrpn

type Handler struct {
	cache [16][4]uint8 // channel -> [cc0,cc1,valcc0,valcc1], initial value [-1,-1,-1,-1]

	// RPN deals with Registered Program Numbers (RPN) and their values.
	// If the callbacks are set, the corresponding control change messages will not be passed of ControlChange.Each.
	RPN struct {

		// MSB is called, when the MSB of a RPN arrives
		MSB func(channel, typ1, typ2, msbVal uint8) (handled bool)

		// LSB is called, when the MSB of a RPN arrives
		LSB func(channel, typ1, typ2, lsbVal uint8) (handled bool)

		// Increment is called, when the increment of a RPN arrives
		Increment func(channel, typ1, typ2 uint8) (handled bool)

		// Decrement is called, when the decrement of a RPN arrives
		Decrement func(channel, typ1, typ2 uint8) (handled bool)

		// Reset is called, when the reset or null RPN arrives
		Reset func(channel uint8) (handled bool)
	}

	// NRPN deals with Non-Registered Program Numbers (NRPN) and their values.
	// If the callbacks are set, the corresponding control change messages will not be passed of ControlChange.Each.
	NRPN struct {

		// MSB is called, when the MSB of a NRPN arrives
		MSB func(channel uint8, typ1, typ2, msbVal uint8) (handled bool)

		// LSB is called, when the LSB of a NRPN arrives
		LSB func(channel uint8, typ1, typ2, lsbVal uint8) (handled bool)

		// Increment is called, when the increment of a NRPN arrives
		Increment func(channel, typ1, typ2 uint8) (handled bool)

		// Decrement is called, when the decrement of a NRPN arrives
		Decrement func(channel, typ1, typ2 uint8) (handled bool)

		// Reset is called, when the reset or null NRPN arrives
		Reset func(channel uint8) (handled bool)
	}
}

func (me *Handler) reset(ch uint8, isRPN bool) (handled bool) {
	// reset tracking on this channel
	me.cache[ch] = [4]uint8{VAL_UNSET, VAL_UNSET, VAL_UNSET, VAL_UNSET}

	if isRPN {
		if me.RPN.Reset != nil {
			return me.RPN.Reset(ch)
		}

		return false
	}

	if me.NRPN.Reset != nil {
		return me.NRPN.Reset(ch)
	}

	return false
}

func (me *Handler) hasRPNCallback() bool {
	return !(me.RPN.MSB == nil && me.RPN.LSB == nil)
}

func (me *Handler) hasNRPNCallback() bool {
	return !(me.NRPN.MSB == nil && me.NRPN.LSB == nil)
}

func (me *Handler) hasNoRPNorNRPNCallback() bool {
	return !me.hasRPNCallback() && !me.hasNRPNCallback()
}

// ReadCCMessage reads a controller message, eventually resulting in a complete rpn / nrpn message.
// handled is only true, if the rpn/nrpn was completed and handled, so while the message is being composed, handled is false.
// This allows a simply way to pass the "not handled" data to the next handler (for a different channel of rpn/nrpn type).
func (me *Handler) ReadCCMessage(ch, cc, val uint8) (handled bool) {

	switch cc {

	/*
		Ok, lets explain the reasoning behind this confusing RPN/NRPN handling a bit.
		There are the following observations:
			- a channel can either have a RPN message or a NRPN message at a point in time
			- the identifiers are sent via CC101 + CC100 for RPN and CC99 + CC98 for NRPN
		    - the order of the identifier CC messages may vary in reality
			- the identifiers are sent before the value
			- the MSB is sent via CC6
			- the LSB is sent via CC38

		RPN and NRPN are never mixed at the same time on the same channel.
		We want to always send complete valid RPN/NRPN messages to the callbacks.
		For this to happen, each identifier is cached and when the MSB arrives and both identifiers are there,
		the callback is called. If any of the conditions are not met, the callback is not called.
	*/

	// first identifier of a RPN/NRPN message
	case CC_RPN0, CC_NRPN0:
		if (cc == CC_RPN0 && !me.hasRPNCallback()) ||
			(cc == CC_NRPN0 && !me.hasNRPNCallback()) {
			return false
		}

		// RPN reset (127,127)
		if val+me.cache[ch][3] == 2*VAL_SET {
			return me.reset(ch, cc == CC_RPN0)
		} else {
			// register first ident cc
			me.cache[ch][0] = cc
			// track the first ident value
			me.cache[ch][2] = val
		}

	// second identifier of a RPN/NRPN message
	case CC_RPN1, CC_NRPN1:
		if (cc == CC_RPN1 && !me.hasRPNCallback()) ||
			(cc == CC_NRPN1 && !me.hasNRPNCallback()) {
			return false
		}

		// RPN reset (127,127)
		if val+me.cache[ch][2] == 2*VAL_SET {
			return me.reset(ch, cc == CC_RPN1)
		} else {
			// register second ident cc
			me.cache[ch][1] = cc
			// track the second ident value
			me.cache[ch][3] = val
		}

	// the data entry controller
	case CC_MSB:
		if me.hasNoRPNorNRPNCallback() {
			return false
		}
		switch {

		// is a valid RPN
		case me.cache[ch][0] == CC_RPN0 && me.cache[ch][1] == CC_RPN1:
			if me.RPN.MSB != nil {
				return me.RPN.MSB(ch, me.cache[ch][2], me.cache[ch][3], val)
			}

		// is a valid NRPN
		case me.cache[ch][0] == CC_NRPN0 && me.cache[ch][1] == CC_NRPN1:
			if me.NRPN.MSB != nil {
				return me.NRPN.MSB(ch, me.cache[ch][2], me.cache[ch][3], val)
			}

		}

	// the lsb
	case CC_LSB:
		if me.hasNoRPNorNRPNCallback() {
			return false
		}

		switch {

		// is a valid RPN
		case me.cache[ch][0] == CC_RPN0 && me.cache[ch][1] == CC_RPN1:
			if me.RPN.LSB != nil {
				return me.RPN.LSB(ch, me.cache[ch][2], me.cache[ch][3], val)
			}

		// is a valid NRPN
		case me.cache[ch][0] == CC_NRPN0 && me.cache[ch][1] == CC_NRPN1:
			if me.NRPN.LSB != nil {
				return me.NRPN.LSB(ch, me.cache[ch][2], me.cache[ch][3], val)
			}

		}

	// the increment
	case CC_INC:
		if me.RPN.Increment == nil && me.NRPN.Increment == nil {
			return false
		}

		switch {

		// is a valid RPN
		case me.cache[ch][0] == CC_RPN0 && me.cache[ch][1] == CC_RPN1:
			if me.RPN.Increment != nil {
				return me.RPN.Increment(ch, me.cache[ch][2], me.cache[ch][3])
			}

		// is a valid NRPN
		case me.cache[ch][0] == CC_NRPN0 && me.cache[ch][1] == CC_NRPN1:
			if me.NRPN.Increment != nil {
				return me.NRPN.Increment(ch, me.cache[ch][2], me.cache[ch][3])
			}

		}

	// the decrement
	case CC_DEC:
		if me.RPN.Decrement == nil && me.NRPN.Decrement == nil {
			return false
		}

		switch {
		// is a valid RPN
		case me.cache[ch][0] == CC_RPN0 && me.cache[ch][1] == CC_RPN1:
			if me.RPN.Decrement != nil {
				return me.RPN.Decrement(ch, me.cache[ch][2], me.cache[ch][3])
			}

		// is a valid NRPN
		case me.cache[ch][0] == CC_NRPN0 && me.cache[ch][1] == CC_NRPN1:
			if me.NRPN.Decrement != nil {
				return me.NRPN.Decrement(ch, me.cache[ch][2], me.cache[ch][3])
			}
		}
	}

	return false
}
