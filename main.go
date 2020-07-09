package main

import (
	"github.com/fuskovic/audio-recorder/cmd"
	"go.coder.com/cli"
)

func main() {
	cli.RunRoot(&cmd.Root{})
}
