package mp3_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"pipelined.dev/audio/mp3"
	"pipelined.dev/pipe"
)

const (
	bufferSize = 512
	mp3Samples = 332928
	sample     = "_testdata/sample.mp3"
	out        = "_testdata/out"
)

func TestMp3(t *testing.T) {
	tests := []struct {
		inFile      string
		bitRateMode mp3.BitRateMode
		channelMode mp3.ChannelMode
		quality     mp3.EncodingQuality
	}{
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.CBR(320),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.CBR(192),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.ABR(220),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.ABR(128),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.VBR(0),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.VBR(9),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			bitRateMode: mp3.VBR(9),
			quality:     mp3.DefaultEncodingQuality,
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			bitRateMode: mp3.VBR(9),
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.VBR(0),
			quality:     0,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			bitRateMode: mp3.VBR(0),
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.Stereo,
			bitRateMode: mp3.VBR(0),
			quality:     3,
		},
	}

	for i, test := range tests {
		t.Logf("Test: %d of %d VBR: %d\n", i+1, len(tests), test.bitRateMode)
		inFile, _ := os.Open(test.inFile)

		outFile, _ := os.Create(fmt.Sprintf("%s-%d-%s.mp3", out, i, test.bitRateMode))

		p, err := pipe.New(
			bufferSize,
			pipe.Line{
				Source: mp3.Source(inFile),
				Sink: mp3.Sink(
					outFile,
					test.bitRateMode,
					test.channelMode,
					test.quality,
				),
			},
		)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		err = pipe.Wait(p.Start(context.Background()))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		_ = inFile.Close()
		_ = outFile.Close()
	}
}
