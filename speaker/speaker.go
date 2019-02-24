// Package speaker implements playback of beep.Streamer values through physical speakers.
package speaker

import (
	"sync"

	"github.com/faiface/beep"
	"github.com/hajimehoshi/oto"
	"github.com/pkg/errors"
)

type Speaker struct {
	mu      sync.Mutex
	mixer   beep.Mixer
	samples [][2]float64
	buf     []byte
	player  *oto.Player
	done    chan struct{}
}

// Init initializes audio playback through speaker. Must be called before using this package.
//
// The bufferSize argument specifies the number of samples of the speaker's buffer. Bigger
// bufferSize means lower CPU usage and more reliable playback. Lower bufferSize means better
// responsiveness and less delay.
func Init(sampleRate beep.SampleRate, bufferSize int) ( *Speaker, error ) {
	speaker := &Speaker{ }

	speaker.mu.Lock()
	defer speaker.mu.Unlock()

	if speaker.player != nil {
		speaker.done <- struct{}{}
		speaker.player.Close()
	}

	speaker.mixer = beep.Mixer{}

	numBytes := bufferSize * 4
	speaker.samples = make([][2]float64, bufferSize)
	speaker.buf = make([]byte, numBytes)

	var err error
	speaker.player, err = oto.NewPlayer(int(sampleRate), 2, 2, numBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize speaker")
	}

	speaker.done = make(chan struct{})

	go func() {
		for {
			select {
			default:
				speaker.update()
			case <-speaker.done:
				return
			}
		}
	}()

	return speaker, nil
}

// Lock locks the speaker. While locked, speaker won't pull new data from the playing Stramers. Lock
// if you want to modify any currently playing Streamers to avoid race conditions.
//
// Always lock speaker for as little time as possible, to avoid playback glitches.
func (speaker *Speaker) Lock() {
	speaker.mu.Lock()
}

// Unlock unlocks the speaker. Call after modifying any currently playing Streamer.
func (speaker *Speaker) Unlock() {
	speaker.mu.Unlock()
}

// Play starts playing all provided Streamers through the speaker.
func (speaker *Speaker) Play(s ...beep.Streamer) {
	speaker.mu.Lock()
	speaker.mixer.Play(s...)
	speaker.mu.Unlock()
}

// Clear removes all currently playing Streamers from the speaker.
func (speaker *Speaker) Clear() {
	speaker.mu.Lock()
	speaker.mixer.Clear()
	speaker.mu.Unlock()
}

// update pulls new data from the playing Streamers and sends it to the speaker. Blocks until the
// data is sent and started playing.
func (speaker *Speaker) update() {
	speaker.mu.Lock()
	speaker.mixer.Stream(speaker.samples)
	speaker.mu.Unlock()

	for i := range speaker.samples {
		for c := range speaker.samples[i] {
			val := speaker.samples[i][c]
			if val < -1 {
				val = -1
			}
			if val > +1 {
				val = +1
			}
			valInt16 := int16(val * (1<<15 - 1))
			low := byte(valInt16)
			high := byte(valInt16 >> 8)
			speaker.buf[i*4+c*2+0] = low
			speaker.buf[i*4+c*2+1] = high
		}
	}

	speaker.player.Write(speaker.buf)
}
