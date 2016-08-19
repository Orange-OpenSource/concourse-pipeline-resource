package main

import (
	"fmt"
	"log"
	"os"

	"github.com/concourse/baggageclaim/fs"
	"github.com/jessevdk/go-flags"
	"github.com/pivotal-golang/lager"
)

type FSMounterCommand struct {
	DiskImage string `long:"disk-image" required:"true" description:"Location of the backing file to create for the image."`

	MountPath string `long:"mount-path" required:"true" description:"Directory where the filesystem should be mounted."`

	SizeInMegabytes uint64 `long:"size-in-megabytes" default:"0" description:"Maximum size of the filesystem. Can exceed the size of the backing device."`

	Remove bool `long:"remove" default:"false" description:"Remove the filesystem instead of creating it."`
}

func main() {
	cmd := &FSMounterCommand{}

	parser := flags.NewParser(cmd, flags.Default)
	parser.NamespaceDelimiter = "-"

	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}

	logger := lager.NewLogger("baggageclaim")
	sink := lager.NewWriterSink(os.Stdout, lager.DEBUG)
	logger.RegisterSink(sink)

	filesystem := fs.New(logger, cmd.DiskImage, cmd.MountPath)

	if !cmd.Remove {
		if cmd.SizeInMegabytes == 0 {
			fmt.Fprintln(os.Stderr, "--size-in-megabytes or --remove must be specified")
			os.Exit(1)
		}

		err := filesystem.Create(cmd.SizeInMegabytes * 1024 * 1024)
		if err != nil {
			log.Fatalln("failed to create filesystem: ", err)
		}
	} else {
		err := filesystem.Delete()
		if err != nil {
			log.Fatalln("failed to delete filesystem: ", err)
		}
	}
}
