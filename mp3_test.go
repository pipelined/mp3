package mp3_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/pipelined/mp3"
	"github.com/stretchr/testify/assert"
)

const (
	bufferSize = 512
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
		inFile, err := os.Open(test.inFile)
		assert.Nil(t, err)
		pump := mp3.Pump{Reader: inFile}

		outFile, err := os.Create(fmt.Sprintf("%s_%d_%s.mp3", out, i, test.vbr))
		assert.Nil(t, err)
		sink := &mp3.Sink{
			Writer:      outFile,
			ChannelMode: test.channelMode,
			BitRateMode: test.vbr,
		}
		if test.useQuality {
			sink.SetQuality(test.quality)
		}

		pumpFn, sampleRate, numChannles, err := pump.Pump("", bufferSize)
		assert.NotNil(t, pumpFn)
		assert.Nil(t, err)

		sinkFn, err := sink.Sink("", sampleRate, numChannles, bufferSize)
		assert.NotNil(t, sinkFn)
		assert.Nil(t, err)

		var buf [][]float64
		messages, samples := 0, 0
		for err == nil {
			buf, err = pumpFn()
			_ = sinkFn(buf)
			messages++
			if buf != nil {
				samples += len(buf[0])
			}
		}

		err = sink.Flush("")
		assert.Nil(t, err)

		err = inFile.Close()
		assert.Nil(t, err)
		err = outFile.Close()
		assert.Nil(t, err)
	}
}
