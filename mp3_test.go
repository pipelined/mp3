package mp3_test

import (
	"testing"

	"github.com/pipelined/mp3"
	"github.com/stretchr/testify/assert"
)

const (
	bufferSize = 512
	sample     = "_testdata/sample.mp3"
	out1       = "_testdata/out1.mp3"
	out2       = "_testdata/out2.mp3"
)

func TestMp3New(t *testing.T) {

	tests := []struct {
		inFile  string
		outFile string
	}{
		{
			inFile:  sample,
			outFile: out1,
		},
		{
			inFile:  out1,
			outFile: out2,
		},
	}

	for _, test := range tests {
		pump := mp3.NewPump(test.inFile)
		sink := mp3.NewSink(test.outFile, 192, 2)

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

		err = pump.Flush("")
		assert.Nil(t, err)
		err = sink.Flush("")
		assert.Nil(t, err)
	}
}
