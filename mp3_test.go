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
	sample     = "_testdata/65.mp3"
	out        = "_testdata/out"
)

func TestMp3(t *testing.T) {

	tests := []struct {
		inFile string
		// outFile string
		vbr     mp3.BitRateMode
		bitRate int
	}{
		{
			inFile:  sample,
			vbr:     mp3.CBR,
			bitRate: 320,
		},
		{
			inFile:  sample,
			vbr:     mp3.CBR,
			bitRate: 192,
		},
	}

	for i, test := range tests {
		inFile, err := os.Open(test.inFile)
		assert.Nil(t, err)
		pump := mp3.Pump{Reader: inFile}

		outFile, err := os.Create(fmt.Sprintf("%s_%d_%s_.mp3", out, i, test.vbr))
		assert.Nil(t, err)
		sink := mp3.CBRSink{
			Writer:      outFile,
			ChannelMode: mp3.JointStereo,
			BitRate:     test.bitRate,
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
