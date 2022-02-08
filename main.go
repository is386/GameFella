package main

import (
	"github.com/is386/GoBoy/emu"
)

var (
	DEBUG = false
	ROM   = "roms/drmario.gb"
)

func main() {
	gb := emu.NewGameBoy(ROM, DEBUG)
	gb.Run()
}
