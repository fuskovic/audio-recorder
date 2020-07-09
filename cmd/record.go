package cmd

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/spf13/pflag"
	"go.coder.com/cli"
	"go.coder.com/flog"
)

var signals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

type recordCmd struct{ outFile string }

// Spec returns a command spec containing a description of it's usage.
func (cmd *recordCmd) Spec() cli.CommandSpec {
	return cli.CommandSpec{
		Name:  "record",
		Usage: "[flags]",
		Desc:  "Record microphone audio.",
	}
}

// RegisterFlags initializes how a flag set is processed for a particular command.
func (cmd *recordCmd) RegisterFlags(fl *pflag.FlagSet) {
	fl.StringVarP(&cmd.outFile, "out", "o", cmd.outFile, "Name the output file.")
}

// Run starts recording microphone audio and stops when input is received from stdin.
func (cmd *recordCmd) Run(fl *pflag.FlagSet) {
	if cmd.outFile == "" {
		cmd.outFile = fmt.Sprintf("%d.aiff", time.Now().Unix())
	} else {
		cmd.outFile += ".aiff"
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, signals...)

	f, err := os.Create(cmd.outFile)
	if err != nil {
		flog.Error("failed to create %s : %v", cmd.outFile, err)
		fl.Usage()
		return
	}

	defer func() {
		flog.Info("closing %s", cmd.outFile)

		if err := f.Close(); err != nil {
			flog.Error("failed to close %s : %v", cmd.outFile, err)
		} else {
			flog.Success("successfully closed %s", cmd.outFile)
		}
	}()

	flog.Success("successfully created %s", cmd.outFile)

	if err := writeFormChunk(f); err != nil {
		flog.Error("failed to write form chunk : %v", err)
		fl.Usage()
		return
	}

	flog.Success("successfully wrote form chunk")

	if err := writeCommonChunk(f); err != nil {
		flog.Error("failed to write common chunk : %v", err)
		fl.Usage()
		return
	}

	flog.Success("successfully wrote common chunk")

	if err := writeSoundChunk(f); err != nil {
		flog.Error("failed to write sound chunk : %v", err)
		fl.Usage()
		return
	}

	flog.Success("successfully wrote sound chunk")

	numSamples := 0

	defer func() {
		flog.Info("filling in missing sizes")

		totalBytes := 50 * numSamples

		// fill in missing sizes
		_, err = f.Seek(4, 0)
		err = binary.Write(f, binary.BigEndian, int32(totalBytes))
		_, err = f.Seek(22, 0)
		err = binary.Write(f, binary.BigEndian, int32(numSamples))
		_, err = f.Seek(42, 0)
		err = binary.Write(f, binary.BigEndian, int32(4*numSamples+8))

		if err != nil {
			flog.Error("failed to fill in missing sizes : %v", err)
		} else {
			flog.Success("successfully filled in missing sizes.")
		}
	}()

	if err := portaudio.Initialize(); err != nil {
		flog.Error("failed to initialize portaudio : %v", err)
		fl.Usage()
		return
	}

	flog.Success("successfully initialized portaudio")

	defer func() {
		flog.Info("terminating portaudio")

		if err := portaudio.Terminate(); err != nil {
			flog.Error("failed to terminate portaudio : %v", err)
		} else {
			flog.Success("successfully terminated port audio")
		}
	}()

	in := make([]int32, 64)

	stream, err := portaudio.OpenDefaultStream(1, 0, 44100, len(in), in)
	if err != nil {
		flog.Error("failed to open audio stream : %v", err)
		fl.Usage()
		return
	}

	flog.Success("successfully opened audio stream")

	defer func() {
		flog.Info("closing audio stream")

		if err := stream.Close(); err != nil {
			flog.Error("failed to close audio stream : %v", err)
		} else {
			flog.Success("successfully closed audio stream")
		}
	}()

	if err := stream.Start(); err != nil {
		flog.Error("failed to start audio stream : %v", err)
		fl.Usage()
		return
	}

	defer func() {
		flog.Info("stopping audio stream")

		if err := stream.Stop(); err != nil {
			flog.Error("failed to stop audio stream : %v", err)
		} else {
			flog.Success("successfully stopped audio stream")
		}
	}()

	done := make(chan bool, 1)
	flog.Success("successfully started capturing audio")

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			done <- true
		}
	}()

	flog.Info("press enter to stop recording")

recording:
	for {
		select {
		case <-done:
			break recording
		case <-stop:
			return
		default:
			if err := stream.Read(); err != nil {
				flog.Error("failed to read from audio stream : %v", err)
			}

			if err := binary.Write(f, binary.BigEndian, in); err != nil {
				flog.Error("failed to write audio data to file as binary : %v", err)
			}
			numSamples += len(in)
		}
	}
}

func writeFormChunk(f *os.File) error {
	// http://paulbourke.net/dataformats/audio/

	// header
	if _, err := f.WriteString("FORM"); err != nil {
		return err
	}

	// total bytes
	if err := binary.Write(f, binary.BigEndian, int32(0)); err != nil {
		return err
	}

	// header
	if _, err := f.WriteString("AIFF"); err != nil {
		return err
	}

	return nil
}

func writeCommonChunk(f *os.File) error {
	// http://paulbourke.net/dataformats/audio/

	sr := []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}

	// header
	if _, err := f.WriteString("COMM"); err != nil {
		return err
	}
	// size
	if err := binary.Write(f, binary.BigEndian, int32(18)); err != nil {
		return err
	}
	// channels
	if err := binary.Write(f, binary.BigEndian, int16(1)); err != nil {
		return err
	}
	// number of samples
	if err := binary.Write(f, binary.BigEndian, int32(0)); err != nil {
		return err
	}
	// bits per sample
	if err := binary.Write(f, binary.BigEndian, int16(32)); err != nil {
		return err
	}
	//80-bit sample rate 44100
	if _, err := f.Write(sr); err != nil {
		return err
	}
	return nil
}

func writeSoundChunk(f *os.File) error {
	// http://paulbourke.net/dataformats/audio/

	// header
	if _, err := f.WriteString("SSND"); err != nil {
		return err
	}
	// size
	if err := binary.Write(f, binary.BigEndian, int32(0)); err != nil {
		return err
	}
	// offset
	if err := binary.Write(f, binary.BigEndian, int32(0)); err != nil {
		return err
	}
	// block
	if err := binary.Write(f, binary.BigEndian, int32(0)); err != nil {
		return err
	}
	return nil
}
