package emu

import (
	"fmt"

	"github.com/is386/GoBoy/emu/bits"
)

var (
	SP         uint16 = 0xFFFE
	INT_VBLANK        = 0
	INT_LCD           = 1
	INT_TIMER         = 2
	INT_SERIAL        = 3
	INT_JOYPAD        = 4
	INT_ADDR          = map[int]uint16{
		INT_VBLANK: 0x40,
		INT_LCD:    0x48,
		INT_TIMER:  0x50,
		INT_SERIAL: 0x58,
		INT_JOYPAD: 0x60,
	}
)

type CPU struct {
	gb                             *GameBoy
	reg                            *Registers
	flags                          *Flags
	pc, sp                         uint16
	halted, ime, imePending, debug bool
	prepareSpeed, speed            int
}

func NewCPU(gb *GameBoy, isCGB bool, bootEnabled bool, debug bool) *CPU {
	c := &CPU{gb: gb, reg: NewRegisters(isCGB), flags: NewFlags(), sp: SP, debug: debug}
	if !bootEnabled {
		c.pc = 0x100
	}
	return c
}

func (c *CPU) readByte(addr uint16) uint8 {
	return c.gb.mmu.readByte(addr)
}

func (c *CPU) readByteHL() uint8 {
	return c.gb.mmu.readByte(c.reg.getHL())
}

func (c *CPU) writeByte(addr uint16, val uint8) {
	c.gb.mmu.writeByte(addr, val)
}

func (c *CPU) writeByteHL(val uint8) {
	c.writeByte(c.reg.getHL(), val)
}

func (c *CPU) nextByte() uint8 {
	val := c.readByte(c.pc)
	c.pc++
	return val
}

func (c *CPU) nextTwoBytes() uint16 {
	a := c.nextByte()
	b := c.nextByte()
	return (uint16(b) << 8) | uint16(a)
}

func (c *CPU) fetch() uint8 {
	return c.nextByte()
}

func (c *CPU) decode(opcode uint8) (func(*CPU), int) {
	if opcode == 0xCB {
		opcode = c.fetch()
		return CB_INSTRUCTIONS[opcode], CB_CYCLES[opcode]
	}
	return INSTRUCTIONS[opcode], CYCLES[opcode]
}

func (c *CPU) execute() int {
	cyc := 4
	if c.halted {
		return cyc / (c.speed + 1)
	}
	if c.debug {
		c.print()
	}
	opcode := c.fetch()
	instr, cyc := c.decode(opcode)
	instr(c)
	return (cyc * 4) / (c.speed + 1)
}

func (c *CPU) print() {
	fmt.Printf("AF: %04X BC: %04X DE: %04X HL: %04X SP: %04X PC: %04X ROM: %02d (%02X %02X %02X %02X)\n",
		c.reg.getAF(c.flags.getF()), c.reg.getBC(), c.reg.getDE(), c.reg.getHL(),
		c.sp, c.pc, c.gb.cart.GetRomBank(), c.readByte(c.pc), c.readByte(c.pc+1), c.readByte(c.pc+2), c.readByte(c.pc+3))
	fmt.Scanln()
}

func (c *CPU) checkInterrupts() {
	if c.imePending {
		c.ime = true
		c.imePending = false
		return
	}

	if !c.ime && !c.halted {
		return
	}

	intF := c.readByte(0xFF0F) | 0xE0
	intE := c.readByte(0xFFFF)
	if intF > 0 {
		for i := 0; i < 5; i++ {
			if (((intF >> i) & 1) == 1) && (((intE >> i) & 1) == 1) {
				c.doInterrupt(i)
				return
			}
		}
	}
}

func (c *CPU) doInterrupt(i int) {
	if !c.ime && c.halted {
		c.halted = false
		return
	}
	c.ime = false
	c.halted = false
	intF := c.readByte(0xFF0F)
	c.writeByte(0xFF0F, c.res(intF, uint8(i)))
	c.push(c.pc)
	c.pc = INT_ADDR[i]
}

func flip(val uint8) uint8 {
	if val == 1 {
		return 0
	} else {
		return 1
	}
}

func (c *CPU) add8(a uint8, b uint8, cy uint8) uint8 {
	ans := uint16(a) + uint16(b) + uint16(cy)
	c.flags.setZero(uint8(ans) == 0)
	c.flags.N = 0
	c.flags.setHalfCarry(((a & 0xF) + (b & 0xF) + cy) > 0xF)
	c.flags.setCarry(ans > 0xFF)
	return uint8(ans)
}

func (c *CPU) add16(a uint16, b uint16) uint16 {
	ans := uint32(a) + uint32(b)
	c.flags.N = 0
	c.flags.setHalfCarry(uint32(a&0xFFF) > (ans & 0xFFF))
	c.flags.setCarry(ans > 0xFFFF)
	return uint16(ans)
}

func (c *CPU) add16Signed(a uint16, b int8) uint16 {
	ans := uint16(int32(a) + int32(b))
	temp := a ^ uint16(b) ^ ans
	c.flags.Z = 0
	c.flags.N = 0
	c.flags.setHalfCarry((temp & 0x10) == 0x10)
	c.flags.setCarry((temp & 0x100) == 0x100)
	return ans
}

func (c *CPU) sub(a uint8, b uint8, cy uint8) uint8 {
	cy = flip(cy)
	ans := c.add8(a, ^b, cy)
	c.flags.N = 1
	c.flags.H = flip(c.flags.H)
	c.flags.C = flip(c.flags.C)
	return uint8(ans)
}

func (c *CPU) and(a uint8, b uint8) uint8 {
	ans := uint16(a) & uint16(b)
	c.flags.setZero(uint8(ans) == 0)
	c.flags.N = 0
	c.flags.H = 1
	c.flags.C = 0
	return uint8(ans)
}

func (c *CPU) or(a uint8, b uint8) uint8 {
	ans := uint16(a) | uint16(b)
	c.flags.setZero(uint8(ans) == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = 0
	return uint8(ans)
}

func (c *CPU) xor(a uint8, b uint8) uint8 {
	ans := uint16(a) ^ uint16(b)
	c.flags.setZero(uint8(ans) == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = 0
	return uint8(ans)
}

func (c *CPU) cp(a uint8, b uint8) {
	ans := a - b
	c.flags.setZero(uint8(ans) == 0)
	c.flags.N = 1
	c.flags.setHalfCarry((b & 0x0F) > (a & 0x0F))
	c.flags.setCarry(b > a)
}

func (c *CPU) inc8(val uint8) uint8 {
	val++
	c.flags.setZero(val == 0)
	c.flags.N = 0
	c.flags.setHalfCarry((val & 0xF) == 0)
	return val
}

func (c *CPU) dec8(val uint8) uint8 {
	val--
	c.flags.setZero(val == 0)
	c.flags.N = 1
	c.flags.setHalfCarry((val & 0xF) == 0xF)
	return val
}

func (c *CPU) bit(val uint8, u3 uint8) {
	c.flags.Z = flip((val >> u3) & 1)
	c.flags.N = 0
	c.flags.H = 1
}

func (c *CPU) res(val uint8, u3 uint8) uint8 {
	return bits.Reset(val, u3)
}

func (c *CPU) set(val uint8, u3 uint8) uint8 {
	return bits.Set(val, u3)
}

func (c *CPU) jump(addr uint16, cond bool) {
	if cond {
		c.pc = addr
	}
}

func (c *CPU) call(addr uint16, cond bool) {
	if cond {
		c.push(c.pc)
		c.pc = addr
	}
}

func (c *CPU) ret(cond bool) {
	if cond {
		c.pc = c.pop()
	}
}

func (c *CPU) rotateLeft(val uint8) uint8 {
	cy := val >> 7
	ans := (val << 1) | c.flags.C
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = cy
	return ans
}

func (c *CPU) rotateLeftCarry(val uint8) uint8 {
	c.flags.C = val >> 7
	ans := (val << 1) | c.flags.C
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	return ans
}

func (c *CPU) rotateRight(val uint8) uint8 {
	cy := val & 1
	ans := (val >> 1) | (c.flags.C << 7)
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = cy
	return ans
}

func (c *CPU) rotateRightCarry(val uint8) uint8 {
	c.flags.C = val & 1
	ans := (val >> 1) | (c.flags.C << 7)
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	return ans
}

func (c *CPU) shiftLeftArith(val uint8) uint8 {
	ans := (val << 1) & 0xFF
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = val >> 7
	return ans
}

func (c *CPU) shiftRightArith(val uint8) uint8 {
	ans := (val & 128) | (val >> 1)
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = val & 1
	return ans
}

func (c *CPU) shiftRightLogical(val uint8) uint8 {
	ans := val >> 1
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = val & 1
	return ans
}

func (c *CPU) swap(val uint8) uint8 {
	ans := (val << 4) | (val >> 4)
	c.flags.setZero(ans == 0)
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = 0
	return ans
}

func (c *CPU) push(val uint16) {
	c.writeByte(c.sp-1, uint8(val>>8))
	c.writeByte(c.sp-2, uint8(val&0xff))
	c.sp -= 2
}

func (c *CPU) pop() uint16 {
	c.sp += 2
	return ((uint16(c.readByte(c.sp-1)) << 8) | uint16(c.readByte(c.sp-2)))
}

func nop(c *CPU) {
}

func addAB(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.B, 0)
}

func addAC(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.C, 0)
}

func addAD(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.D, 0)
}

func addAE(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.E, 0)
}

func addAH(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.H, 0)
}

func addAL(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.L, 0)
}

func addAHL(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.readByteHL(), 0)
}

func addAA(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.A, 0)
}

func adi(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.nextByte(), 0)
}

func adcAB(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.B, c.flags.C)
}

func adcAC(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.C, c.flags.C)
}

func adcAD(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.D, c.flags.C)
}

func adcAE(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.E, c.flags.C)
}

func adcAH(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.H, c.flags.C)
}

func adcAL(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.L, c.flags.C)
}

func adcAHL(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.readByteHL(), c.flags.C)
}

func adcAA(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.reg.A, c.flags.C)
}

func aci(c *CPU) {
	c.reg.A = c.add8(c.reg.A, c.nextByte(), c.flags.C)
}

func addHLBC(c *CPU) {
	c.reg.setHL(c.add16(c.reg.getHL(), c.reg.getBC()))
}

func addHLDE(c *CPU) {
	c.reg.setHL(c.add16(c.reg.getHL(), c.reg.getDE()))
}

func addHLHL(c *CPU) {
	c.reg.setHL(c.add16(c.reg.getHL(), c.reg.getHL()))
}

func addHLSP(c *CPU) {
	c.reg.setHL(c.add16(c.reg.getHL(), c.sp))
}

func addSP(c *CPU) {
	c.sp = c.add16Signed(c.sp, int8(c.nextByte()))
}

func subAB(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.B, 0)
}

func subAC(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.C, 0)
}

func subAD(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.D, 0)
}

func subAE(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.E, 0)
}

func subAH(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.H, 0)
}

func subAL(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.L, 0)
}

func subAHL(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.readByteHL(), 0)
}

func subAA(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.A, 0)
}

func sui(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.nextByte(), 0)
}

func sbcAB(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.B, c.flags.C)
}

func sbcAC(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.C, c.flags.C)
}

func sbcAD(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.D, c.flags.C)
}

func sbcAE(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.E, c.flags.C)
}

func sbcAH(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.H, c.flags.C)
}

func sbcAL(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.L, c.flags.C)
}

func sbcAHL(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.readByteHL(), c.flags.C)
}

func sbcAA(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.reg.A, c.flags.C)
}

func sbi(c *CPU) {
	c.reg.A = c.sub(c.reg.A, c.nextByte(), c.flags.C)
}

func andAB(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.B)
}

func andAC(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.C)
}

func andAD(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.D)
}

func andAE(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.E)
}

func andAH(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.H)
}

func andAL(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.L)
}

func andAHL(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.readByteHL())
}

func andAA(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.reg.A)
}

func ani(c *CPU) {
	c.reg.A = c.and(c.reg.A, c.nextByte())
}

func orAB(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.B)
}

func orAC(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.C)
}

func orAD(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.D)
}

func orAE(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.E)
}

func orAH(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.H)
}

func orAL(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.L)
}

func orAHL(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.readByteHL())
}

func orAA(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.reg.A)
}

func ori(c *CPU) {
	c.reg.A = c.or(c.reg.A, c.nextByte())
}

func xorAB(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.B)
}

func xorAC(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.C)
}

func xorAD(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.D)
}

func xorAE(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.E)
}

func xorAH(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.H)
}

func xorAL(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.L)
}

func xorAHL(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.readByteHL())
}

func xorAA(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.reg.A)
}

func xri(c *CPU) {
	c.reg.A = c.xor(c.reg.A, c.nextByte())
}

func cpAB(c *CPU) {
	c.cp(c.reg.A, c.reg.B)
}

func cpAC(c *CPU) {
	c.cp(c.reg.A, c.reg.C)
}

func cpAD(c *CPU) {
	c.cp(c.reg.A, c.reg.D)
}

func cpAE(c *CPU) {
	c.cp(c.reg.A, c.reg.E)
}

func cpAH(c *CPU) {
	c.cp(c.reg.A, c.reg.H)
}

func cpAL(c *CPU) {
	c.cp(c.reg.A, c.reg.L)
}

func cpAHL(c *CPU) {
	c.cp(c.reg.A, c.readByteHL())
}

func cpAA(c *CPU) {
	c.cp(c.reg.A, c.reg.A)
}

func cpi(c *CPU) {
	c.cp(c.reg.A, c.nextByte())
}

func cpl(c *CPU) {
	c.reg.A ^= 255
	c.flags.N = 1
	c.flags.H = 1
}

func decB(c *CPU) {
	c.reg.B = c.dec8(c.reg.B)
}

func decC(c *CPU) {
	c.reg.C = c.dec8(c.reg.C)
}

func decD(c *CPU) {
	c.reg.D = c.dec8(c.reg.D)
}

func decE(c *CPU) {
	c.reg.E = c.dec8(c.reg.E)
}

func decH(c *CPU) {
	c.reg.H = c.dec8(c.reg.H)
}

func decL(c *CPU) {
	c.reg.L = c.dec8(c.reg.L)
}

func decHL(c *CPU) {
	c.writeByteHL(c.dec8(c.readByteHL()))
}

func decA(c *CPU) {
	c.reg.A = c.dec8(c.reg.A)
}

func decBC(c *CPU) {
	c.reg.setBC(c.reg.getBC() - 1)
}

func decDE(c *CPU) {
	c.reg.setDE(c.reg.getDE() - 1)
}

func decHL16(c *CPU) {
	c.reg.setHL(c.reg.getHL() - 1)
}

func decSP(c *CPU) {
	c.sp--
}

func incB(c *CPU) {
	c.reg.B = c.inc8(c.reg.B)
}

func incC(c *CPU) {
	c.reg.C = c.inc8(c.reg.C)
}

func incD(c *CPU) {
	c.reg.D = c.inc8(c.reg.D)
}

func incE(c *CPU) {
	c.reg.E = c.inc8(c.reg.E)
}

func incH(c *CPU) {
	c.reg.H = c.inc8(c.reg.H)
}

func incL(c *CPU) {
	c.reg.L = c.inc8(c.reg.L)
}

func incHL(c *CPU) {
	c.writeByteHL(c.inc8(c.readByteHL()))
}

func incA(c *CPU) {
	c.reg.A = c.inc8(c.reg.A)
}

func incBC(c *CPU) {
	c.reg.setBC(c.reg.getBC() + 1)
}

func incDE(c *CPU) {
	c.reg.setDE(c.reg.getDE() + 1)
}

func incHL16(c *CPU) {
	c.reg.setHL(c.reg.getHL() + 1)
}

func incSP(c *CPU) {
	c.sp++
}

func bit0B(c *CPU) {
	c.bit(c.reg.B, 0)
}

func bit0C(c *CPU) {
	c.bit(c.reg.C, 0)
}

func bit0D(c *CPU) {
	c.bit(c.reg.D, 0)
}

func bit0E(c *CPU) {
	c.bit(c.reg.E, 0)
}

func bit0H(c *CPU) {
	c.bit(c.reg.H, 0)
}

func bit0L(c *CPU) {
	c.bit(c.reg.L, 0)
}

func bit0A(c *CPU) {
	c.bit(c.reg.A, 0)
}

func bit1B(c *CPU) {
	c.bit(c.reg.B, 1)
}

func bit1C(c *CPU) {
	c.bit(c.reg.C, 1)
}

func bit1D(c *CPU) {
	c.bit(c.reg.D, 1)
}

func bit1E(c *CPU) {
	c.bit(c.reg.E, 1)
}

func bit1H(c *CPU) {
	c.bit(c.reg.H, 1)
}

func bit1L(c *CPU) {
	c.bit(c.reg.L, 1)
}

func bit1A(c *CPU) {
	c.bit(c.reg.A, 1)
}

func bit2B(c *CPU) {
	c.bit(c.reg.B, 2)
}

func bit2C(c *CPU) {
	c.bit(c.reg.C, 2)
}

func bit2D(c *CPU) {
	c.bit(c.reg.D, 2)
}

func bit2E(c *CPU) {
	c.bit(c.reg.E, 2)
}

func bit2H(c *CPU) {
	c.bit(c.reg.H, 2)
}

func bit2L(c *CPU) {
	c.bit(c.reg.L, 2)
}

func bit2A(c *CPU) {
	c.bit(c.reg.A, 2)
}

func bit3B(c *CPU) {
	c.bit(c.reg.B, 3)
}

func bit3C(c *CPU) {
	c.bit(c.reg.C, 3)
}

func bit3D(c *CPU) {
	c.bit(c.reg.D, 3)
}

func bit3E(c *CPU) {
	c.bit(c.reg.E, 3)
}

func bit3H(c *CPU) {
	c.bit(c.reg.H, 3)
}

func bit3L(c *CPU) {
	c.bit(c.reg.L, 3)
}

func bit3A(c *CPU) {
	c.bit(c.reg.A, 3)
}

func bit4B(c *CPU) {
	c.bit(c.reg.B, 4)
}

func bit4C(c *CPU) {
	c.bit(c.reg.C, 4)
}

func bit4D(c *CPU) {
	c.bit(c.reg.D, 4)
}

func bit4E(c *CPU) {
	c.bit(c.reg.E, 4)
}

func bit4H(c *CPU) {
	c.bit(c.reg.H, 4)
}

func bit4L(c *CPU) {
	c.bit(c.reg.L, 4)
}

func bit4A(c *CPU) {
	c.bit(c.reg.A, 4)
}

func bit5B(c *CPU) {
	c.bit(c.reg.B, 5)
}

func bit5C(c *CPU) {
	c.bit(c.reg.C, 5)
}

func bit5D(c *CPU) {
	c.bit(c.reg.D, 5)
}

func bit5E(c *CPU) {
	c.bit(c.reg.E, 5)
}

func bit5H(c *CPU) {
	c.bit(c.reg.H, 5)
}

func bit5L(c *CPU) {
	c.bit(c.reg.L, 5)
}

func bit5A(c *CPU) {
	c.bit(c.reg.A, 5)
}

func bit6B(c *CPU) {
	c.bit(c.reg.B, 6)
}

func bit6C(c *CPU) {
	c.bit(c.reg.C, 6)
}

func bit6D(c *CPU) {
	c.bit(c.reg.D, 6)
}

func bit6E(c *CPU) {
	c.bit(c.reg.E, 6)
}

func bit6H(c *CPU) {
	c.bit(c.reg.H, 6)
}

func bit6L(c *CPU) {
	c.bit(c.reg.L, 6)
}

func bit6A(c *CPU) {
	c.bit(c.reg.A, 6)
}

func bit7B(c *CPU) {
	c.bit(c.reg.B, 7)
}

func bit7C(c *CPU) {
	c.bit(c.reg.C, 7)
}

func bit7D(c *CPU) {
	c.bit(c.reg.D, 7)
}

func bit7E(c *CPU) {
	c.bit(c.reg.E, 7)
}

func bit7H(c *CPU) {
	c.bit(c.reg.H, 7)
}

func bit7L(c *CPU) {
	c.bit(c.reg.L, 7)
}

func bit7A(c *CPU) {
	c.bit(c.reg.A, 7)
}

func bit0HL(c *CPU) {
	c.bit(c.readByteHL(), 0)
}

func bit1HL(c *CPU) {
	c.bit(c.readByteHL(), 1)
}

func bit2HL(c *CPU) {
	c.bit(c.readByteHL(), 2)
}

func bit3HL(c *CPU) {
	c.bit(c.readByteHL(), 3)
}

func bit4HL(c *CPU) {
	c.bit(c.readByteHL(), 4)
}

func bit5HL(c *CPU) {
	c.bit(c.readByteHL(), 5)
}

func bit6HL(c *CPU) {
	c.bit(c.readByteHL(), 6)
}

func bit7HL(c *CPU) {
	c.bit(c.readByteHL(), 7)
}

func res0B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 0)
}

func res0C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 0)
}

func res0D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 0)
}

func res0E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 0)
}

func res0H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 0)
}

func res0L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 0)
}

func res0A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 0)
}

func res1B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 1)
}

func res1C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 1)
}

func res1D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 1)
}

func res1E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 1)
}

func res1H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 1)
}

func res1L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 1)
}

func res1A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 1)
}

func res2B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 2)
}

func res2C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 2)
}

func res2D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 2)
}

func res2E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 2)
}

func res2H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 2)
}

func res2L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 2)
}

func res2A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 2)
}

func res3B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 3)
}

func res3C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 3)
}

func res3D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 3)
}

func res3E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 3)
}

func res3H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 3)
}

func res3L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 3)
}

func res3A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 3)
}

func res4B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 4)
}

func res4C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 4)
}

func res4D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 4)
}

func res4E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 4)
}

func res4H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 4)
}

func res4L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 4)
}

func res4A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 4)
}

func res5B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 5)
}

func res5C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 5)
}

func res5D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 5)
}

func res5E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 5)
}

func res5H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 5)
}

func res5L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 5)
}

func res5A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 5)
}

func res6B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 6)
}

func res6C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 6)
}

func res6D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 6)
}

func res6E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 6)
}

func res6H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 6)
}

func res6L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 6)
}

func res6A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 6)
}

func res7B(c *CPU) {
	c.reg.B = c.res(c.reg.B, 7)
}

func res7C(c *CPU) {
	c.reg.C = c.res(c.reg.C, 7)
}

func res7D(c *CPU) {
	c.reg.D = c.res(c.reg.D, 7)
}

func res7E(c *CPU) {
	c.reg.E = c.res(c.reg.E, 7)
}

func res7H(c *CPU) {
	c.reg.H = c.res(c.reg.H, 7)
}

func res7L(c *CPU) {
	c.reg.L = c.res(c.reg.L, 7)
}

func res7A(c *CPU) {
	c.reg.A = c.res(c.reg.A, 7)
}

func res0HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 0))
}

func res1HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 1))
}

func res2HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 2))
}

func res3HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 3))
}

func res4HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 4))
}

func res5HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 5))
}

func res6HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 6))
}

func res7HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.res(c.readByteHL(), 7))
}

func set0B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 0)
}

func set0C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 0)
}

func set0D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 0)
}

func set0E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 0)
}

func set0H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 0)
}

func set0L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 0)
}

func set0A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 0)
}

func set1B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 1)
}

func set1C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 1)
}

func set1D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 1)
}

func set1E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 1)
}

func set1H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 1)
}

func set1L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 1)
}

func set1A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 1)
}

func set2B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 2)
}

func set2C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 2)
}

func set2D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 2)
}

func set2E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 2)
}

func set2H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 2)
}

func set2L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 2)
}

func set2A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 2)
}

func set3B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 3)
}

func set3C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 3)
}

func set3D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 3)
}

func set3E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 3)
}

func set3H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 3)
}

func set3L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 3)
}

func set3A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 3)
}

func set4B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 4)
}

func set4C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 4)
}

func set4D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 4)
}

func set4E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 4)
}

func set4H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 4)
}

func set4L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 4)
}

func set4A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 4)
}

func set5B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 5)
}

func set5C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 5)
}

func set5D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 5)
}

func set5E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 5)
}

func set5H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 5)
}

func set5L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 5)
}

func set5A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 5)
}

func set6B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 6)
}

func set6C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 6)
}

func set6D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 6)
}

func set6E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 6)
}

func set6H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 6)
}

func set6L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 6)
}

func set6A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 6)
}

func set7B(c *CPU) {
	c.reg.B = c.set(c.reg.B, 7)
}

func set7C(c *CPU) {
	c.reg.C = c.set(c.reg.C, 7)
}

func set7D(c *CPU) {
	c.reg.D = c.set(c.reg.D, 7)
}

func set7E(c *CPU) {
	c.reg.E = c.set(c.reg.E, 7)
}

func set7H(c *CPU) {
	c.reg.H = c.set(c.reg.H, 7)
}

func set7L(c *CPU) {
	c.reg.L = c.set(c.reg.L, 7)
}

func set7A(c *CPU) {
	c.reg.A = c.set(c.reg.A, 7)
}

func set0HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 0))
}

func set1HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 1))
}

func set2HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 2))
}

func set3HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 3))
}

func set4HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 4))
}

func set5HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 5))
}

func set6HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 6))
}

func set7HL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.set(c.readByteHL(), 7))
}

func swapB(c *CPU) {
	c.reg.B = c.swap(c.reg.B)
}

func swapC(c *CPU) {
	c.reg.C = c.swap(c.reg.C)
}

func swapD(c *CPU) {
	c.reg.D = c.swap(c.reg.D)
}

func swapE(c *CPU) {
	c.reg.E = c.swap(c.reg.E)
}

func swapH(c *CPU) {
	c.reg.H = c.swap(c.reg.H)
}

func swapL(c *CPU) {
	c.reg.L = c.swap(c.reg.L)
}

func swapHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.swap(b))
}

func swapA(c *CPU) {
	c.reg.A = c.swap(c.reg.A)
}

func ldBB(c *CPU) {
}

func ldBC(c *CPU) {
	c.reg.B = c.reg.C
}

func ldBD(c *CPU) {
	c.reg.B = c.reg.D
}

func ldBE(c *CPU) {
	c.reg.B = c.reg.E
}

func ldBH(c *CPU) {
	c.reg.B = c.reg.H
}

func ldBL(c *CPU) {
	c.reg.B = c.reg.L
}

func ldBA(c *CPU) {
	c.reg.B = c.reg.A
}

func ldCB(c *CPU) {
	c.reg.C = c.reg.B
}

func ldCC(c *CPU) {}

func ldCD(c *CPU) {
	c.reg.C = c.reg.D
}

func ldCE(c *CPU) {
	c.reg.C = c.reg.E
}

func ldCH(c *CPU) {
	c.reg.C = c.reg.H
}

func ldCL(c *CPU) {
	c.reg.C = c.reg.L
}

func ldCA(c *CPU) {
	c.reg.C = c.reg.A
}

func ldDB(c *CPU) {
	c.reg.D = c.reg.B
}

func ldDC(c *CPU) {
	c.reg.D = c.reg.C
}

func ldDD(c *CPU) {}

func ldDE(c *CPU) {
	c.reg.D = c.reg.E
}

func ldDH(c *CPU) {
	c.reg.D = c.reg.H
}

func ldDL(c *CPU) {
	c.reg.D = c.reg.L
}

func ldDA(c *CPU) {
	c.reg.D = c.reg.A
}

func ldEB(c *CPU) {
	c.reg.E = c.reg.B
}

func ldEC(c *CPU) {
	c.reg.E = c.reg.C
}

func ldED(c *CPU) {
	c.reg.E = c.reg.D
}

func ldEE(c *CPU) {}

func ldEH(c *CPU) {
	c.reg.E = c.reg.H
}

func ldEL(c *CPU) {
	c.reg.E = c.reg.L
}

func ldEA(c *CPU) {
	c.reg.E = c.reg.A
}

func ldHB(c *CPU) {
	c.reg.H = c.reg.B
}

func ldHC(c *CPU) {
	c.reg.H = c.reg.C
}

func ldHD(c *CPU) {
	c.reg.H = c.reg.D
}

func ldHE(c *CPU) {
	c.reg.H = c.reg.E
}

func ldHH(c *CPU) {}

func ldHL(c *CPU) {
	c.reg.H = c.reg.L
}

func ldHA(c *CPU) {
	c.reg.H = c.reg.A
}

func ldLB(c *CPU) {
	c.reg.L = c.reg.B
}

func ldLC(c *CPU) {
	c.reg.L = c.reg.C
}

func ldLD(c *CPU) {
	c.reg.L = c.reg.D
}

func ldLE(c *CPU) {
	c.reg.L = c.reg.E
}

func ldLH(c *CPU) {
	c.reg.L = c.reg.H
}

func ldLL(c *CPU) {}

func ldLA(c *CPU) {
	c.reg.L = c.reg.A
}

func ldAB(c *CPU) {
	c.reg.A = c.reg.B
}

func ldAC(c *CPU) {
	c.reg.A = c.reg.C
}

func ldAD(c *CPU) {
	c.reg.A = c.reg.D
}

func ldAE(c *CPU) {
	c.reg.A = c.reg.E
}

func ldAH(c *CPU) {
	c.reg.A = c.reg.H
}

func ldAL(c *CPU) {
	c.reg.A = c.reg.L
}

func ldAA(c *CPU) {}

func ldiB(c *CPU) {
	c.reg.B = c.nextByte()
}

func ldiC(c *CPU) {
	c.reg.C = c.nextByte()
}

func ldiD(c *CPU) {
	c.reg.D = c.nextByte()
}

func ldiE(c *CPU) {
	c.reg.E = c.nextByte()
}

func ldiH(c *CPU) {
	c.reg.H = c.nextByte()
}

func ldiL(c *CPU) {
	c.reg.L = c.nextByte()
}

func ldiA(c *CPU) {
	c.reg.A = c.nextByte()
}

func ldBC16(c *CPU) {
	c.reg.setBC(c.nextTwoBytes())
}

func ldDE16(c *CPU) {
	c.reg.setDE(c.nextTwoBytes())
}

func ldHL16(c *CPU) {
	c.reg.setHL(c.nextTwoBytes())
}

func ldHLB(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.B)
}

func ldHLC(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.C)
}

func ldHLD(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.D)
}

func ldHLE(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.E)
}

func ldHLH(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.H)
}

func ldHLL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.L)
}

func ldiHL(c *CPU) {
	c.writeByte(c.reg.getHL(), c.nextByte())
}

func ldBHL(c *CPU) {
	c.reg.B = c.readByteHL()
}

func ldCHL(c *CPU) {
	c.reg.C = c.readByteHL()
}

func ldDHL(c *CPU) {
	c.reg.D = c.readByteHL()
}

func ldEHL(c *CPU) {
	c.reg.E = c.readByteHL()
}

func ldHHL(c *CPU) {
	c.reg.H = c.readByteHL()
}

func ldLHL(c *CPU) {
	c.reg.L = c.readByteHL()
}

func ldBCA(c *CPU) {
	c.writeByte(c.reg.getBC(), c.reg.A)
}

func ldDEA(c *CPU) {
	c.writeByte(c.reg.getDE(), c.reg.A)
}

func ldHLA(c *CPU) {
	c.writeByte(c.reg.getHL(), c.reg.A)
}

func ld16A(c *CPU) {
	c.writeByte(c.nextTwoBytes(), c.reg.A)
}

func ldh16A(c *CPU) {
	addr := uint16(c.nextByte()) + 0xFF00
	if addr <= 0xFFFF {
		c.writeByte(addr, c.reg.A)
	}
}

func ldhCA(c *CPU) {
	addr := uint16(c.reg.C) + 0xFF00
	if addr <= 0xFFFF {
		c.writeByte(addr, c.reg.A)
	}
}

func ldABC(c *CPU) {
	c.reg.A = c.readByte(c.reg.getBC())
}

func ldADE(c *CPU) {
	c.reg.A = c.readByte(c.reg.getDE())
}

func ldAHL(c *CPU) {
	c.reg.A = c.readByteHL()
}

func ldA16(c *CPU) {
	c.reg.A = c.readByte(c.nextTwoBytes())
}

func ldhA16(c *CPU) {
	addr := uint16(c.nextByte()) + 0xFF00
	if addr <= 0xFFFF {
		c.reg.A = c.readByte(addr)
	}
}

func ldhAC(c *CPU) {
	addr := uint16(c.reg.C) + 0xFF00
	if addr <= 0xFFFF {
		c.reg.A = c.readByte(addr)
	}
}

func ldHLIA(c *CPU) {
	ldHLA(c)
	c.reg.setHL(c.reg.getHL() + 1)
}

func ldHLDA(c *CPU) {
	ldHLA(c)
	c.reg.setHL(c.reg.getHL() - 1)
}

func ldAHLI(c *CPU) {
	ldAHL(c)
	c.reg.setHL(c.reg.getHL() + 1)
}

func ldAHLD(c *CPU) {
	ldAHL(c)
	c.reg.setHL(c.reg.getHL() - 1)
}

func ldSP16(c *CPU) {
	c.sp = c.nextTwoBytes()
}

func ld16SP(c *CPU) {
	addr := c.nextTwoBytes()
	c.writeByte(addr, uint8(c.sp&0xFF))
	c.writeByte(addr+1, uint8(c.sp>>8))
}

func ldHLSP(c *CPU) {
	c.reg.setHL(c.add16Signed(c.sp, int8(c.nextByte())))
}

func ldSPHL(c *CPU) {
	c.sp = c.reg.getHL()
}

func jp(c *CPU) {
	c.jump(c.nextTwoBytes(), true)
}

func jpz(c *CPU) {
	c.jump(c.nextTwoBytes(), c.flags.Z == 1)
}

func jpnz(c *CPU) {
	c.jump(c.nextTwoBytes(), c.flags.Z == 0)
}

func jpc(c *CPU) {
	c.jump(c.nextTwoBytes(), c.flags.C == 1)
}

func jpnc(c *CPU) {
	c.jump(c.nextTwoBytes(), c.flags.C == 0)
}

func jpHL(c *CPU) {
	c.jump(c.reg.getHL(), true)
}

func jr(c *CPU) {
	e8 := int8(c.nextByte())
	c.jump(uint16(int(c.pc)+int(e8)), true)
}

func jrz(c *CPU) {
	e8 := int8(c.nextByte())
	c.jump(uint16(int(c.pc)+int(e8)), c.flags.Z == 1)
}

func jrnz(c *CPU) {
	e8 := int8(c.nextByte())
	c.jump(uint16(int(c.pc)+int(e8)), c.flags.Z == 0)
}

func jrc(c *CPU) {
	e8 := int8(c.nextByte())
	c.jump(uint16(int(c.pc)+int(e8)), c.flags.C == 1)
}

func jrnc(c *CPU) {
	e8 := int8(c.nextByte())
	c.jump(uint16(int(c.pc)+int(e8)), c.flags.C == 0)
}

func call(c *CPU) {
	c.call(c.nextTwoBytes(), true)
}

func callz(c *CPU) {
	c.call(c.nextTwoBytes(), c.flags.Z == 1)
}

func callnz(c *CPU) {
	c.call(c.nextTwoBytes(), c.flags.Z == 0)
}

func callc(c *CPU) {
	c.call(c.nextTwoBytes(), c.flags.C == 1)
}

func callnc(c *CPU) {
	c.call(c.nextTwoBytes(), c.flags.C == 0)
}

func rst0(c *CPU) {
	c.call(0x00, true)
}

func rst8(c *CPU) {
	c.call(0x08, true)
}

func rst10(c *CPU) {
	c.call(0x10, true)
}

func rst18(c *CPU) {
	c.call(0x18, true)
}

func rst20(c *CPU) {
	c.call(0x20, true)
}

func rst28(c *CPU) {
	c.call(0x28, true)
}

func rst30(c *CPU) {
	c.call(0x30, true)
}

func rst38(c *CPU) {
	c.call(0x38, true)
}

func ret(c *CPU) {
	c.ret(true)
}

func retz(c *CPU) {
	c.ret(c.flags.Z == 1)
}

func retnz(c *CPU) {
	c.ret(c.flags.Z == 0)
}

func retc(c *CPU) {
	c.ret(c.flags.C == 1)
}

func retnc(c *CPU) {
	c.ret(c.flags.C == 0)
}

func reti(c *CPU) {
	c.ret(true)
	c.ime = true
}

func popAF(c *CPU) {
	af := c.pop()
	c.reg.A = uint8(af >> 8)
	psw := uint8(af & 0xff)
	c.flags.Z = (psw >> 7) & 1
	c.flags.N = (psw >> 6) & 1
	c.flags.H = (psw >> 5) & 1
	c.flags.C = (psw >> 4) & 1
}

func popBC(c *CPU) {
	c.reg.setBC(c.pop())
}

func popDE(c *CPU) {
	c.reg.setDE(c.pop())
}

func popHL(c *CPU) {
	c.reg.setHL(c.pop())
}

func pushAF(c *CPU) {
	c.push((uint16(c.reg.A) << 8) | uint16(c.flags.getF()))
}

func pushBC(c *CPU) {
	c.push(c.reg.getBC())
}

func pushDE(c *CPU) {
	c.push(c.reg.getDE())
}

func pushHL(c *CPU) {
	c.push(c.reg.getHL())
}

func rlB(c *CPU) {
	c.reg.B = c.rotateLeft(c.reg.B)
}

func rlC(c *CPU) {
	c.reg.C = c.rotateLeft(c.reg.C)
}

func rlD(c *CPU) {
	c.reg.D = c.rotateLeft(c.reg.D)
}

func rlE(c *CPU) {
	c.reg.E = c.rotateLeft(c.reg.E)
}

func rlH(c *CPU) {
	c.reg.H = c.rotateLeft(c.reg.H)
}

func rlL(c *CPU) {
	c.reg.L = c.rotateLeft(c.reg.L)
}

func rlHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.rotateLeft(b))
}

func rlA(c *CPU) {
	c.reg.A = c.rotateLeft(c.reg.A)
}

func rla(c *CPU) {
	c.reg.A = c.rotateLeft(c.reg.A)
	c.flags.Z = 0
}

func rlcB(c *CPU) {
	c.reg.B = c.rotateLeftCarry(c.reg.B)
}

func rlcC(c *CPU) {
	c.reg.C = c.rotateLeftCarry(c.reg.C)
}

func rlcD(c *CPU) {
	c.reg.D = c.rotateLeftCarry(c.reg.D)
}

func rlcE(c *CPU) {
	c.reg.E = c.rotateLeftCarry(c.reg.E)
}

func rlcH(c *CPU) {
	c.reg.H = c.rotateLeftCarry(c.reg.H)
}

func rlcL(c *CPU) {
	c.reg.L = c.rotateLeftCarry(c.reg.L)
}

func rlcHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.rotateLeftCarry(b))
}

func rlcA(c *CPU) {
	c.reg.A = c.rotateLeftCarry(c.reg.A)
}

func rlca(c *CPU) {
	c.reg.A = c.rotateLeftCarry(c.reg.A)
	c.flags.Z = 0
}

func rrB(c *CPU) {
	c.reg.B = c.rotateRight(c.reg.B)
}

func rrC(c *CPU) {
	c.reg.C = c.rotateRight(c.reg.C)
}

func rrD(c *CPU) {
	c.reg.D = c.rotateRight(c.reg.D)
}

func rrE(c *CPU) {
	c.reg.E = c.rotateRight(c.reg.E)
}

func rrH(c *CPU) {
	c.reg.H = c.rotateRight(c.reg.H)
}

func rrL(c *CPU) {
	c.reg.L = c.rotateRight(c.reg.L)
}

func rrHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.rotateRight(b))
}

func rrA(c *CPU) {
	c.reg.A = c.rotateRight(c.reg.A)
}

func rra(c *CPU) {
	c.reg.A = c.rotateRight(c.reg.A)
	c.flags.Z = 0
}

func rrcB(c *CPU) {
	c.reg.B = c.rotateRightCarry(c.reg.B)
}

func rrcC(c *CPU) {
	c.reg.C = c.rotateRightCarry(c.reg.C)
}

func rrcD(c *CPU) {
	c.reg.D = c.rotateRightCarry(c.reg.D)
}

func rrcE(c *CPU) {
	c.reg.E = c.rotateRightCarry(c.reg.E)
}

func rrcH(c *CPU) {
	c.reg.H = c.rotateRightCarry(c.reg.H)
}

func rrcL(c *CPU) {
	c.reg.L = c.rotateRightCarry(c.reg.L)
}

func rrcHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.rotateRightCarry(b))
}

func rrcA(c *CPU) {
	c.reg.A = c.rotateRightCarry(c.reg.A)
}

func rrca(c *CPU) {
	c.reg.A = c.rotateRightCarry(c.reg.A)
	c.flags.Z = 0
}

func slaB(c *CPU) {
	c.reg.B = c.shiftLeftArith(c.reg.B)
}

func slaC(c *CPU) {
	c.reg.C = c.shiftLeftArith(c.reg.C)
}

func slaD(c *CPU) {
	c.reg.D = c.shiftLeftArith(c.reg.D)
}

func slaE(c *CPU) {
	c.reg.E = c.shiftLeftArith(c.reg.E)
}

func slaH(c *CPU) {
	c.reg.H = c.shiftLeftArith(c.reg.H)
}

func slaL(c *CPU) {
	c.reg.L = c.shiftLeftArith(c.reg.L)
}

func slaHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.shiftLeftArith(b))
}

func slaA(c *CPU) {
	c.reg.A = c.shiftLeftArith(c.reg.A)
}

func sraB(c *CPU) {
	c.reg.B = c.shiftRightArith(c.reg.B)
}

func sraC(c *CPU) {
	c.reg.C = c.shiftRightArith(c.reg.C)
}

func sraD(c *CPU) {
	c.reg.D = c.shiftRightArith(c.reg.D)
}

func sraE(c *CPU) {
	c.reg.E = c.shiftRightArith(c.reg.E)
}

func sraH(c *CPU) {
	c.reg.H = c.shiftRightArith(c.reg.H)
}

func sraL(c *CPU) {
	c.reg.L = c.shiftRightArith(c.reg.L)
}

func sraHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.shiftRightArith(b))
}

func sraA(c *CPU) {
	c.reg.A = c.shiftRightArith(c.reg.A)
}

func srlB(c *CPU) {
	c.reg.B = c.shiftRightLogical(c.reg.B)
}

func srlC(c *CPU) {
	c.reg.C = c.shiftRightLogical(c.reg.C)
}

func srlD(c *CPU) {
	c.reg.D = c.shiftRightLogical(c.reg.D)
}

func srlE(c *CPU) {
	c.reg.E = c.shiftRightLogical(c.reg.E)
}

func srlH(c *CPU) {
	c.reg.H = c.shiftRightLogical(c.reg.H)
}

func srlL(c *CPU) {
	c.reg.L = c.shiftRightLogical(c.reg.L)
}

func srlHL(c *CPU) {
	b := c.readByteHL()
	c.writeByte(c.reg.getHL(), c.shiftRightLogical(b))
}

func srlA(c *CPU) {
	c.reg.A = c.shiftRightLogical(c.reg.A)
}

func daa(c *CPU) {
	if c.flags.N == 0 {
		if c.flags.C == 1 || c.reg.A > 0x99 {
			c.reg.A += 0x60
			c.flags.C = 1
		}
		if c.flags.H == 1 || ((c.reg.A & 0xF) > 0x9) {
			c.reg.A += 0x06
			c.flags.H = 0
		}
	} else if c.flags.C == 1 && c.flags.H == 1 {
		c.reg.A += 0x9A
		c.flags.H = 0
	} else if c.flags.C == 1 {
		c.reg.A += 0xA0
	} else if c.flags.H == 1 {
		c.reg.A += 0xFA
		c.flags.H = 0
	}
	c.flags.setZero(c.reg.A == 0)
}

func di(c *CPU) {
	c.ime = false
}

func ei(c *CPU) {
	c.imePending = true
}

func scf(c *CPU) {
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = 1
}

func ccf(c *CPU) {
	c.flags.N = 0
	c.flags.H = 0
	c.flags.C = flip(c.flags.C)
}

func halt(c *CPU) {
	c.halted = true
}

func stop(c *CPU) {
	c.halted = true
	if !c.gb.isCGB {
		return
	}
	if c.prepareSpeed == 1 {
		c.prepareSpeed = 0
		if c.speed == 1 {
			c.speed = 0
		} else {
			c.speed = 1
		}
		c.halted = false
	}
	c.fetch()
}
