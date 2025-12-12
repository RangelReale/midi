package player

import (
	"context"
	"errors"
	"io"
	"runtime"
	"sort"
	"sync"
	"time"

	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/smf"
)

/*
stolen from https://github.com/Minoxs/gomidi-player
*/

type (
	// message is an smf.Message to be played, followed by the sleeping time to the next message
	message struct {
		msg   smf.Message
		sleep time.Duration
	}

	// Player plays SMF data
	Player struct {
		mutex      sync.RWMutex
		isPlaying  bool
		ctx        context.Context
		cancelFn   context.CancelCauseFunc
		currentDur time.Duration
		totalDur   time.Duration
		currentMsg int
		messages   []message
		outPort    drivers.Out
	}
)

// New returns a Player that plays on the given output port
func New(outPort drivers.Out) *Player {
	return &Player{
		ctx:      UnavailableContext(),
		cancelFn: func(cause error) {},
		outPort:  outPort,
	}
}

// stop will signal the player to stop playing
func (me *Player) stop(cause error) error {
	if !me.isPlaying {
		return ErrIsStopped
	}
	me.cancelFn(cause)
	return nil
}

// SetSMF takes in a SMF and creates units that can be played.
func (me *Player) SetSMF(smfdata io.Reader, tracks ...int) error {
	me.mutex.Lock()
	defer me.mutex.Unlock()

	if me.isPlaying {
		return ErrIsPlaying
	}

	me.currentMsg = 0
	me.currentDur = 0
	me.messages = nil

	var events smfReader
	err := events.read(smfdata, tracks...)
	if err != nil {
		return err
	}

	me.messages, me.totalDur = events.getMessages()
	return nil
}

// Start starts playing. It is non-blocking. Call wait to wait until it is finished.
func (me *Player) Start() error {
	me.mutex.Lock()
	defer me.mutex.Unlock()

	if me.messages == nil {
		return ErrNoSMFData
	}

	if me.isPlaying {
		return ErrIsPlaying
	}

	me.isPlaying = true
	me.ctx, me.cancelFn = context.WithCancelCause(context.Background())

	go me.playOn(me.outPort)
	return nil
}

// Stop stops the playing.
func (me *Player) Stop() (err error) {
	return me.stop(errStopped)
}

// Pause pauses the playing.
func (me *Player) Pause() (err error) {
	return me.stop(errPaused)
}

// Wait will block until the player finishes playing.
func (me *Player) Wait() {
	for me.isPlaying {
		time.Sleep(50 * time.Millisecond)
	}
}

// IsPlaying returns wether the player is playing
func (me *Player) IsPlaying() bool {
	return me.isPlaying
}

// Duration is the total duration of the song.
func (me *Player) Duration() time.Duration {
	return me.totalDur
}

// Current is the current time on the song.
func (me *Player) Current() time.Duration {
	return me.currentDur
}

// Remaining is the remaining duration of the smf data.
func (me *Player) Remaining() time.Duration {
	return me.totalDur - me.currentDur
}

type smfReader struct {
	trackEvents []smf.TrackEvent
}

// read reads SMF data and parses all the track events.
func (me *smfReader) read(smfdata io.Reader, tracks ...int) (err error) {
	me.trackEvents = make([]smf.TrackEvent, 0, 100)
	return WrapOnError(
		smf.ReadTracksFrom(smfdata, tracks...).Do(me.readEvent).Error(),
		ErrInvalidSMF,
	)
}

// readEvent reads a single event from the track.
func (me *smfReader) readEvent(event smf.TrackEvent) {
	if event.Message.IsPlayable() {
		me.trackEvents = append(me.trackEvents, event)
	}
}

// getMessages parses the track events and returns the playable units.
func (me *smfReader) getMessages() (messages []message, totalDur time.Duration) {
	me.sortTrackEvents()
	messages = make([]message, len(me.trackEvents))
	totalDur = 0
	for i := 0; i < len(messages); i++ {
		var event = me.trackEvents[i]
		messages[i] = message{
			msg:   event.Message,
			sleep: time.Microsecond * time.Duration(event.AbsMicroSeconds-totalDur.Microseconds()),
		}
		totalDur += messages[i].sleep
	}
	return
}

// sortTrackEvents makes sure the song is ordered by time.
func (me *smfReader) sortTrackEvents() {
	sort.SliceStable(
		me.trackEvents, func(i, j int) bool {
			return me.trackEvents[i].AbsMicroSeconds < me.trackEvents[j].AbsMicroSeconds
		},
	)
}

// playOn will play the current song in the given out port
func (me *Player) playOn(out drivers.Out) {
	defer me.cleanupAfterPlaying()

	// Drivers may invoke CGO
	// Makes sure thread is locked to avoid weird errors
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	//	defer IgnoreError(out.Close)

	// Creates timer to properly time sound writes
	var sleep = time.NewTimer(0)
	defer sleep.Stop()

	// Makes sure channel is drained
	<-sleep.C

	// play all messages
	for i, m := range me.messages[me.currentMsg:] {
		me.currentDur += m.sleep
		me.currentMsg = i
		if m.sleep > 0 {
			sleep.Reset(m.sleep)
			select {
			case <-sleep.C:
				break
			case <-me.ctx.Done():
				return
			}
		}
		_ = out.Send(m.msg)
	}
}

// cleanupAfterPlaying does the cleaning up after playOn ends
func (me *Player) cleanupAfterPlaying() {
	_ = me.stop(errDone)
	var err = context.Cause(me.ctx)

	switch {
	case errors.Is(err, errDone):
		fallthrough
	case errors.Is(err, errStopped):
		me.currentMsg = 0
		me.currentDur = 0
	}

	me.isPlaying = false
}

func Play(out drivers.Out, smfdata io.Reader, tracks ...int) (*Player, error) {
	player := New(out)
	err := player.SetSMF(smfdata, tracks...)
	if err != nil {
		return nil, err
	}

	err = player.Start()
	if err != nil {
		return nil, err
	}

	player.Wait()
	return player, nil
}
