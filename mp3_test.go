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
		vbr         mp3.BitRateMode
		channelMode mp3.ChannelMode
		useQuality  bool
		quality     int
	}{
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.CBR(320),
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.CBR(192),
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.ABR(220),
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.ABR(128),
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR(0),
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR(9),
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			vbr:         mp3.VBR(9),
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			vbr:         mp3.VBR(9),
			useQuality:  true,
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR(0),
			useQuality:  true,
			quality:     0,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR(0),
			useQuality:  true,
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.Stereo,
			vbr:         mp3.VBR(0),
			useQuality:  true,
			quality:     3,
		},
	}

	for i, test := range tests {
		t.Logf("Test: %d of %d VBR: %d\n", i+1, len(tests), test.vbr)
		inFile, _ := os.Open(test.inFile)
		pumpAllocator := mp3.Pump{Reader: inFile}

		outFile, _ := os.Create(fmt.Sprintf("%s-%d-%s.mp3", out, i, test.vbr))
		sinkAllocator := &mp3.Sink{
			Writer:      outFile,
			ChannelMode: test.channelMode,
			BitRateMode: test.vbr,
		}
		if test.useQuality {
			sinkAllocator.SetQuality(test.quality)
		}

		line, _ := pipe.Routing{
			Source: pumpAllocator.Pump(),
			Sink:   sinkAllocator.Sink(),
		}.Line(bufferSize)
		p := pipe.New(context.Background(), pipe.WithLines(line))
		err := p.Wait()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		_ = inFile.Close()
		_ = outFile.Close()
	}
}
