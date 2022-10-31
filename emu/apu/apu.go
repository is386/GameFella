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

	FPS         = 60
	SAMPLE_RATE = 44100
	SAMPLES     = 8192
	CLOCK_SPEED = 4194304
	FRAMETIME   = time.Second / time.Duration(FPS)
	CYCLES      = 8192
)

type APU struct {
	c1            *Channel1
	c2            *Channel2
	c3            *Channel3
	c4            *Channel4
	cyc           int
	frameSequence int
	sampleCounter int
	player        *oto.Player
	buffer        chan [2]uint8
	volLeft       uint8
	volRight      uint8
}

func NewAPU() *APU {
	apu := &APU{cyc: CYCLES}
	apu.c1 = NewChannel1()
	apu.c2 = NewChannel2()
	apu.c3 = NewChannel3()
	apu.c4 = NewChannel4()
	apu.buffer = make(chan [2]uint8, SAMPLES)

	ctx, err := oto.NewContext(SAMPLE_RATE, 2, 1, SAMPLE_RATE/FPS)
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
		var reading [2]byte
		for range ticker.C {
			fbLen := len(a.buffer)
			buffer := make([]byte, fbLen*2)
			for i := 0; i < fbLen*2; i += 2 {
				reading = <-a.buffer
				if reading[0] == 0 && reading[1] == 0 {
					continue
				}
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
		a.cyc = CYCLES
		switch a.frameSequence {
		case 0:
			a.c1.clockLength()
			a.c2.clockLength()
			a.c3.clockLength()
			a.c4.clockLength()
		case 2:
			a.c1.clockSweep()
			a.c1.clockLength()
			a.c2.clockLength()
			a.c3.clockLength()
			a.c4.clockLength()
		case 4:
			a.c1.clockLength()
			a.c2.clockLength()
			a.c3.clockLength()
			a.c4.clockLength()
		case 6:
			a.c1.clockSweep()
			a.c1.clockLength()
			a.c2.clockLength()
			a.c3.clockLength()
			a.c4.clockLength()
		case 7:
			a.c1.clockEnvelope()
			a.c2.clockEnvelope()
			a.c4.clockEnvelope()
		}
		a.frameSequence++
		a.frameSequence &= 7
	}
}

func (a *APU) updateChannels() {
	a.c1.update()
	a.c2.update()
	a.c3.update()
	a.c4.update()
}

func (a *APU) playSound() {
	a.sampleCounter += SAMPLE_RATE
	if a.sampleCounter >= CLOCK_SPEED {
		a.sampleCounter -= CLOCK_SPEED

		sampleL := a.c1.left + a.c2.left + a.c3.left + a.c4.left
		sampleR := a.c1.right + a.c2.right + a.c3.right + a.c4.right

		l := uint8(sampleL * int(a.volLeft/2))
		r := uint8(sampleR * int(a.volRight/2))

		a.buffer <- [2]uint8{l, r}
	}
}

func (a *APU) ReadByte(addr uint16) uint8 {
	switch uint8(addr & 0x00FF) {
	case NR10, NR11, NR12, NR13, NR14:
		return a.c1.readByte(uint8(addr & 0x00FF))

	case NR21, NR22, NR23, NR24:
		return a.c2.readByte(uint8(addr & 0x00FF))

	case NR30, NR31, NR32, NR33, NR34:
		return a.c3.readByte(uint8(addr & 0x00FF))

	case NR41, NR42, NR43, NR44:
		return a.c4.readByte(uint8(addr & 0x00FF))
	}

	if addr >= 0xFF30 && addr <= 0xFF3F {
		return a.c3.readByte(uint8(addr & 0x00FF))
	}

	return 0x00
}

func (a *APU) WriteByte(addr uint16, val uint8) {
	switch uint8(addr & 0x00FF) {

	case NR10, NR11, NR12, NR13, NR14:
		a.c1.writeByte(uint8(addr&0x00FF), val)

	case NR21, NR22, NR23, NR24:
		a.c2.writeByte(uint8(addr&0x00FF), val)

	case NR30, NR31, NR32, NR33, NR34:
		a.c3.writeByte(uint8(addr&0x00FF), val)

	case NR41, NR42, NR43, NR44:
		a.c4.writeByte(uint8(addr&0x00FF), val)

	case NR50:
		a.volLeft = (val >> 4) & 0x7
		a.volRight = val & 0x7

	case NR51:
		a.c1.leftOn = bits.Value(val, 4)
		a.c2.leftOn = bits.Value(val, 5)
		a.c3.leftOn = bits.Value(val, 6)
		a.c4.leftOn = bits.Value(val, 7)

		a.c1.rightOn = bits.Value(val, 0)
		a.c2.rightOn = bits.Value(val, 1)
		a.c3.rightOn = bits.Value(val, 2)
		a.c4.rightOn = bits.Value(val, 3)
	}

	if addr >= 0xFF30 && addr <= 0xFF3F {
		a.c3.writeByte(uint8(addr&0x00FF), val)
	}
}
