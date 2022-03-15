package main

// TODO:
// - Pass blarggs timing tests
// - Pass DMG and CGB sound tests
// - Implement NR52
// - Super GameBoy
// - Save States
// - Rewind

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
	"github.com/is386/GoBoy/emu"
	"github.com/sqweek/dialog"
)

func parseArgs() (string, int, bool) {
	parser := argparse.NewParser("GameFella", "A simple GameBoy emulator written in Go.")

	bootFlag := parser.String("b", "boot",
		&argparse.Options{
			Required: false,
			Help:     "Path to boot ROM",
			Default:  "",
		})

	scaleFlag := parser.Int("s", "scale",
		&argparse.Options{
			Required: false,
			Help:     "Scale of the screen",
			Default:  3,
		})

	debugFlag := parser.Flag("d", "debug",
		&argparse.Options{
			Required: false,
			Help:     "Turns on debugging mode",
			Default:  false,
		})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(0)
	}

	return *bootFlag, *scaleFlag, *debugFlag
}

func main() {
	bootPath, scale, debug := parseArgs()
	rom, err := dialog.File().Filter("GB/GBC Rom File", "gb", "gbc").Load()
	if err != nil {
		panic(err)
	}
	gb := emu.NewGameBoy(rom, bootPath, scale, debug)
	gb.Run()
}
