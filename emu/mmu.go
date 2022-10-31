package emu

import (
	"github.com/is386/GoBoy/emu/bits"
)

var (
	JOYPAD uint8 = 0x00
	COMM1  uint8 = 0x01
	COMM2  uint8 = 0x02
	DIV    uint8 = 0x04
	TIMA   uint8 = 0x05
	TMA    uint8 = 0x06
	TAC    uint8 = 0x07
	LCDC   uint8 = 0x40
	STAT   uint8 = 0x41
	SCY    uint8 = 0x42
	SCX    uint8 = 0x43
	LY     uint8 = 0x44
	LYC    uint8 = 0x45
	DMA    uint8 = 0x46
	BGP    uint8 = 0x47
	OBP0   uint8 = 0x48
	OBP1   uint8 = 0x49
	WY     uint8 = 0x4A
	WX     uint8 = 0x4B
	VBK    uint8 = 0x4F
	KEY1   uint8 = 0x4D
	HDMA1  uint8 = 0x51
	HDMA2  uint8 = 0x52
	HDMA3  uint8 = 0x53
	HDMA4  uint8 = 0x54
	HDMA5  uint8 = 0x55
	BGPI   uint8 = 0x68
	BGPD   uint8 = 0x69
	OBPI   uint8 = 0x6A
	OBPD   uint8 = 0x6B
	OPRI   uint8 = 0x6C
	SVBK   uint8 = 0x70
)

type MMU struct {
	gb               *GameBoy
	bootROM          []uint8
	VRAM             [2][0x2000]uint8
	WRAM0            [0x1000]uint8
	WRAM             [8][0x1000]uint8
	OAM              [0x00A0]uint8
	HRAM             [0x0100]uint8
	vramBank         uint8
	wramBank         uint8
	bgCRAM           CRAM
	spriteCRAM       CRAM
	bootEnabled      bool
	bootJustDisabled bool
	hdmaActive       bool
}

func NewMMU(gb *GameBoy) *MMU {
	m := MMU{gb: gb, wramBank: 1}
	m.initHRAM()
	return &m
}

func (m *MMU) initHRAM() {
	m.HRAM[0x00] = 0xCF
	m.HRAM[0x01] = 0x00
	m.HRAM[0x02] = 0x7E
	m.HRAM[0x04] = 0xAB
	m.HRAM[0x05] = 0x00
	m.HRAM[0x06] = 0x00
	m.HRAM[0x07] = 0xF8
	m.HRAM[0x0F] = 0xE1
	m.HRAM[0x10] = 0x80
	m.HRAM[0x11] = 0xBF
	m.HRAM[0x12] = 0xF3
	m.HRAM[0x13] = 0xFF
	m.HRAM[0x14] = 0xBF
	m.HRAM[0x16] = 0x3F
	m.HRAM[0x17] = 0x00
	m.HRAM[0x18] = 0xFF
	m.HRAM[0x19] = 0xBF
	m.HRAM[0x1A] = 0x7F
	m.HRAM[0x1B] = 0xFF
	m.HRAM[0x1C] = 0x9F
	m.HRAM[0x1D] = 0xFF
	m.HRAM[0x1E] = 0xBF
	m.HRAM[0x20] = 0xFF
	m.HRAM[0x21] = 0x00
	m.HRAM[0x22] = 0x00
	m.HRAM[0x23] = 0xBF
	m.HRAM[0x24] = 0x77
	m.HRAM[0x25] = 0xF3
	m.HRAM[0x26] = 0xF1
	m.HRAM[0x40] = 0x91
	m.HRAM[0x41] = 0x85
	m.HRAM[0x42] = 0x00
	m.HRAM[0x43] = 0x00
	m.HRAM[0x44] = 0x00
	m.HRAM[0x45] = 0x00
	m.HRAM[0x46] = 0xFF
	m.HRAM[0x47] = 0xFC
	m.HRAM[0x4A] = 0x00
	m.HRAM[0x4B] = 0x00
	m.HRAM[0x4D] = 0xFF
	m.HRAM[0x4F] = 0xFF
	m.HRAM[0x51] = 0xFF
	m.HRAM[0x52] = 0xFF
	m.HRAM[0x53] = 0xFF
	m.HRAM[0x54] = 0xFF
	m.HRAM[0x55] = 0xFF
	m.HRAM[0x56] = 0xFF
	m.HRAM[0x68] = 0xFF
	m.HRAM[0x69] = 0xFF
	m.HRAM[0x6A] = 0xFF
	m.HRAM[0x6B] = 0xFF
	m.HRAM[0x70] = 0xFF
	m.HRAM[0xFF] = 0x00
}

func (m *MMU) loadBootRom(rom []uint8) {
	m.bootROM = rom
	m.bootEnabled = true
}

func (m *MMU) readByte(addr uint16) uint8 {
	switch addr & 0xF000 {
	case 0x0000:
		if m.bootEnabled && (addr < 0x100 || (addr >= 0x200 && addr < 0x900)) {
			return m.bootROM[addr]
		} else if m.bootEnabled && m.gb.cpu.pc == 0x100 {
			m.bootEnabled = false
		}
		return m.gb.cart.ReadByte(addr)

	case 0x1000, 0x2000, 0x3000, 0x4000, 0x5000, 0x6000, 0x7000:
		return m.gb.cart.ReadByte(addr)

	case 0x8000, 0x9000:
		return m.VRAM[m.vramBank][addr-0x8000]

	case 0xA000, 0xB000:
		return m.gb.cart.ReadByte(addr)

	case 0xC000, 0xD000, 0xE000:
		if addr >= 0xE000 {
			addr -= 0x2000
		}
		if addr <= 0xCFFF {
			return m.WRAM0[addr-0xC000]
		} else {
			return m.WRAM[m.wramBank][addr-0xD000]
		}
	}

	switch addr & 0x0F00 {
	case 0x0E00:
		if addr-0xFE00 < 160 {
			return m.OAM[addr-0xFE00]
		}

	case 0x0F00:
		if addr == 0xFF00 {
			return m.gb.buttons.readByte(addr)
		}
		if addr >= 0xFF10 && addr <= 0xFF26 {
			m.gb.apu.ReadByte(addr)
		} else if addr >= 0xFF30 && addr <= 0xFF3F {
			m.gb.apu.ReadByte(addr)
		}
		return m.readHRAM(uint8(addr - 0xFF00))
	}

	return 0xFF
}

func (m *MMU) writeByte(addr uint16, val uint8) {
	switch addr & 0xF000 {

	case 0x0000, 0x1000, 0x2000, 0x3000, 0x4000, 0x5000, 0x6000, 0x7000:
		m.gb.cart.WriteROM(addr, val)
		return

	case 0x8000, 0x9000:
		m.VRAM[m.vramBank][addr-0x8000] = val
		return

	case 0xA000, 0xB000:
		m.gb.cart.WriteRAM(addr, val)
		return

	case 0xC000, 0xD000, 0xE000:
		if addr >= 0xE000 {
			addr -= 0x2000
		}
		if addr <= 0xCFFF {
			m.WRAM0[addr-0xC000] = val
		} else {
			m.WRAM[m.wramBank][addr-0xD000] = val
		}
		return
	}

	switch addr & 0x0F00 {
	case 0x0E00:
		if addr < 0xFEA0 {
			m.OAM[addr-0xFE00] = val
		}

	case 0x0F00:
		if addr >= 0xFF10 && addr <= 0xFF26 {
			m.gb.apu.WriteByte(addr, val)
			return
		} else if addr >= 0xFF30 && addr <= 0xFF3F {
			m.gb.apu.WriteByte(addr, val)
			return
		}
		m.writeHRAM(uint8(addr-0xFF00), val)
	}
}

func (m *MMU) writeHRAM(reg uint8, val uint8) {
	switch reg {
	case JOYPAD:
		m.gb.buttons.writeByte(0xFF00, val)

	case COMM2:
		return

	case DIV:
		m.gb.timer.resetDivCyc()
		m.HRAM[DIV] = 0

	case TAC:
		freq0 := m.gb.timer.getTimerFreq()
		m.HRAM[TAC] = val | 0xF8
		freq := m.gb.timer.getTimerFreq()
		if freq0 != freq {
			m.gb.timer.resetTimer()
		}

	case STAT:
		m.HRAM[STAT] = val | 0x80

	case DMA:
		m.dmaTransfer(val)

	case HDMA5:
		if m.gb.isCGB {
			m.newDMATransfer(val)
		}

	case VBK:
		if m.gb.isCGB && !m.hdmaActive {
			m.vramBank = val & 1
		}

	case SVBK:
		if m.gb.isCGB {
			m.wramBank = val & 0x7
			if m.wramBank == 0 {
				m.wramBank = 1
			}
		}

	case KEY1:
		if m.gb.isCGB {
			m.gb.cpu.prepareSpeed = int(bits.Value(val, 0))
		}

	case BGPI:
		if m.gb.isCGB {
			m.bgCRAM.writeIndex(val)
		}

	case BGPD:
		if m.gb.isCGB {
			m.bgCRAM.writeCRAM(val)
		}

	case OBPI:
		if m.gb.isCGB {
			m.spriteCRAM.writeIndex(val)
		}

	case OBPD:
		if m.gb.isCGB {
			m.spriteCRAM.writeCRAM(val)
		}

	default:
		m.HRAM[reg] = val
	}
}

func (m *MMU) readBgCRAM(addr uint8) uint8 {
	return m.bgCRAM.readCRAM(addr)
}

func (m *MMU) readSpriteCRAM(addr uint8) uint8 {
	return m.spriteCRAM.readCRAM(addr)
}

func (m *MMU) readVRAM(addr uint16, bank uint8) uint8 {
	return m.VRAM[bank][addr-0x8000]
}

func (m *MMU) readHRAM(reg uint8) uint8 {
	switch reg {

	case VBK:
		return m.vramBank

	case SVBK:
		return m.wramBank

	case KEY1:
		return uint8(m.gb.cpu.speed<<7) | uint8(m.gb.cpu.prepareSpeed)

	case BGPI:
		return m.bgCRAM.index

	case BGPD:
		return m.bgCRAM.readCurrentCRAM()

	case OBPI:
		return m.spriteCRAM.index

	case OBPD:
		return m.spriteCRAM.readCurrentCRAM()

	default:
		return m.HRAM[reg]
	}
}

func (m *MMU) dmaTransfer(val uint8) {
	addr := uint16(val) << 8
	for i := uint16(0); i < 0xA0; i++ {
		m.writeByte(0xFE00+i, m.readByte(addr+i))
	}
}

func (m *MMU) newDMATransfer(val uint8) {
	mode := bits.Value(val, 7)

	if mode == 1 && !m.hdmaActive {
		m.hdmaActive = true
		m.HRAM[HDMA5] = val & 0x7F
	} else if m.hdmaActive && mode == 0 {
		m.hdmaActive = false
		m.HRAM[HDMA5] |= 0x80
	} else {
		len := ((uint16(val) & 0x7F) + 1) * 16
		m.gdmaTransfer(len)
		m.HRAM[HDMA5] = 0xFF
	}
}

func (m *MMU) gdmaTransfer(len uint16) {
	src := ((uint16(m.HRAM[HDMA1]) << 8) | uint16(m.HRAM[HDMA2])) & 0xFFF0
	dst := ((uint16(m.HRAM[HDMA3]) << 8) | uint16(m.HRAM[HDMA4])) & 0x1FF0

	for i := uint16(0); i < len; i++ {
		m.VRAM[m.vramBank][dst] = m.readByte(src)
		src++
		dst++
	}
	src++
	dst++

	m.HRAM[HDMA1] = uint8(src >> 8)
	m.HRAM[HDMA2] = uint8((src & 0xFF))
	m.HRAM[HDMA3] = uint8((dst >> 8))
	m.HRAM[HDMA4] = uint8(dst & 0xF0)
}

func (m *MMU) hdmaTransfer() {
	if !m.hdmaActive {
		return
	}

	m.gdmaTransfer(16)
	m.HRAM[HDMA5]--
	if m.HRAM[HDMA5] == 0xFF {
		m.hdmaActive = false
	}
}

func (m *MMU) writeInterrupt(i int) {
	req := m.HRAM[0x0F] | 0xE0
	req = bits.Set(req, uint8(i))
	m.writeByte(0xFF0F, req)
}

func (m *MMU) incrDiv() {
	m.HRAM[DIV]++
}

func (m *MMU) incrLY() {
	m.HRAM[LY]++
}

func (m *MMU) incrTima() {
	m.HRAM[TIMA]++
}
