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
		bitRate     int
		vbrQuality  int
		useQuality  bool
		quality     int
	}{
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.CBR,
			bitRate:     320,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.CBR,
			bitRate:     192,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.ABR,
			bitRate:     220,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.ABR,
			bitRate:     128,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR,
			vbrQuality:  0,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR,
			vbrQuality:  9,
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			vbr:         mp3.VBR,
			vbrQuality:  9,
		},
		{
			inFile:      sample,
			channelMode: mp3.Mono,
			vbr:         mp3.VBR,
			vbrQuality:  9,
			useQuality:  true,
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR,
			vbrQuality:  0,
			useQuality:  true,
			quality:     0,
		},
		{
			inFile:      sample,
			channelMode: mp3.JointStereo,
			vbr:         mp3.VBR,
			vbrQuality:  0,
			useQuality:  true,
			quality:     9,
		},
		{
			inFile:      sample,
			channelMode: mp3.Stereo,
			vbr:         mp3.VBR,
			vbrQuality:  0,
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
		var sink mp3.Sink
		switch test.vbr {
		case mp3.CBR:
			sink = &mp3.CBRSink{
				Writer:      outFile,
				ChannelMode: test.channelMode,
				BitRate:     test.bitRate,
			}
		case mp3.ABR:
			sink = &mp3.ABRSink{
				Writer:      outFile,
				ChannelMode: test.channelMode,
				BitRate:     test.bitRate,
			}
		case mp3.VBR:
			sink = &mp3.VBRSink{
				Writer:      outFile,
				ChannelMode: test.channelMode,
				VBRQuality:  test.vbrQuality,
			}
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
