package apu

import (
	"time"

	"github.com/hajimehoshi/oto"
	"github.com/is386/GoBoy/emu/bits"
)

var (
	NR10 uint8 = 0x10
	NR11 uint8 = 0x11
	NR12 uint8 = 0x12
	NR13 uint8 = 0x13
	NR14 uint8 = 0x14
	NR21 uint8 = 0x16
	NR22 uint8 = 0x17
	NR23 uint8 = 0x18
	NR24 uint8 = 0x19
	NR30 uint8 = 0x1A
	NR31 uint8 = 0x1B
	NR32 uint8 = 0x1C
	NR33 uint8 = 0x1D
	NR34 uint8 = 0x1E
	NR41 uint8 = 0x20
	NR42 uint8 = 0x21
	NR43 uint8 = 0x22
	NR44 uint8 = 0x23
	NR50 uint8 = 0x24
	NR51 uint8 = 0x25
	NR52 uint8 = 0x26

	SAMPLE_RATE = 48000
	BUFFER_SIZE = 2048
	CLOCK_SPEED = 4194304
	FRAMETIME   = time.Second / 60
)

type APU struct {
	c2            *Channel2
	cyc           int
	frameSequence int
	sampleCounter int
	player        *oto.Player
	buffer        chan [2]uint8
	volLeft       uint8
	volRight      uint8
}

func NewAPU() *APU {
	apu := &APU{cyc: 8192}
	apu.c2 = NewChannel2()
	apu.buffer = make(chan [2]uint8, BUFFER_SIZE)

	ctx, err := oto.NewContext(SAMPLE_RATE, 2, 1, BUFFER_SIZE)
	if err != nil {
		panic(err)
	}

	apu.player = ctx.NewPlayer()
	apu.startSoundRoutine()
	return apu
}

func (a *APU) startSoundRoutine() {
	ticker := time.NewTicker(FRAMETIME)
	go func() {
		var reading [2]uint8
		for range ticker.C {
			bufLen := len(a.buffer)
			buffer := make([]uint8, bufLen*2)
			for i := 0; i < bufLen*2; i += 2 {
				reading = <-a.buffer
				buffer[i], buffer[i+1] = reading[0], reading[1]
			}
			a.player.Write(buffer)
		}
	}()
}

func (a *APU) Update(cyc int) {
	for i := 0; i < cyc; i++ {
		a.frameSequencer()
		a.updateChannels()
		a.playSound()
	}
}

func (a *APU) frameSequencer() {
	a.cyc--
	if a.cyc <= 0 {
		a.cyc = 8192
		switch a.frameSequence {
		case 0:
			a.c2.clockLength()
		case 2:
			a.c2.clockLength()
		case 4:
			a.c2.clockLength()
		case 6:
			a.c2.clockLength()
		case 7:
			a.c2.clockEnvelope()
		}
		a.frameSequence++
		a.frameSequence &= 7
	}
}

func (a *APU) updateChannels() {
	a.c2.update()
}

func (a *APU) playSound() {
	a.sampleCounter += SAMPLE_RATE
	if a.sampleCounter >= CLOCK_SPEED {
		a.sampleCounter -= CLOCK_SPEED
		sampleL := a.c2.left
		sampleR := a.c2.right
		a.buffer <- [2]uint8{uint8(sampleL * uint16(a.volLeft)), uint8(sampleR * uint16(a.volRight))}
	}
}

func (a *APU) ReadByte(addr uint16) uint8 {
	switch uint8(addr & 0x00FF) {
	case NR21, NR22, NR23, NR24:
		return a.c2.readByte(uint8(addr & 0x00FF))
	}
	return 0x00
}

func (a *APU) WriteByte(addr uint16, val uint8) {
	switch uint8(addr & 0x00FF) {
	case NR21, NR22, NR23, NR24:
		a.c2.writeByte(uint8(addr&0x00FF), val)
	case NR50:
		a.volLeft = (val >> 4) & 0x7
		a.volRight = val & 0x7
	case NR51:
		a.c2.rightOn = bits.Value(val, 1)
		a.c2.leftOn = bits.Value(val, 5)
	}
}
