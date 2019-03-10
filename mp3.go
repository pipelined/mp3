package mp3

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/viert/lame"

	"github.com/pipelined/signal"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// Pump allows to read mp3 files.
// This component cannot be reused for consequent runs.
type Pump struct {
	r io.ReadCloser
	d *mp3.Decoder
}

// NewPump creates new mp3 Pump.
func NewPump(r io.ReadCloser) *Pump {
	return &Pump{
		r: r,
	}
}

// Pump reads buffer from mp3.
func (p *Pump) Pump(sourceID string, bufferSize int) (func() ([][]float64, error), int, int, error) {
	var err error

	p.d, err = mp3.NewDecoder(p.r)
	if err != nil {
		return nil, 0, 0, err
	}

	// current decoder always provides stereo, so constant
	numChannels := 2
	sampleRate := p.d.SampleRate()

	return func() ([][]float64, error) {
		capacity := bufferSize * numChannels
		ints := make([]int, 0, capacity)

		var val int16
		for len(ints) < capacity {
			if err := binary.Read(p.d, binary.LittleEndian, &val); err != nil { // read next frame
				if err == io.EOF { // no bytes available
					if len(ints) == 0 {
						return nil, io.EOF
					}
					break
				} else { // error happened
					return nil, err
				}
			} else {
				ints = append(ints, int(val)) // append data
			}
		}

		b := signal.InterInt{Data: ints, NumChannels: numChannels, BitDepth: signal.BitDepth16}.AsFloat64()
		// read not enough samples
		if b.Size() != bufferSize {
			return b, io.ErrUnexpectedEOF
		}
		return b, nil
	}, sampleRate, numChannels, nil
}

// Sink allows to send data to mp3 files.
type Sink struct {
	w       io.Writer
	e       *lame.LameWriter
	bitRate int
	quality int
}

// NewSink creates new Sink.
func NewSink(w io.Writer, bitRate int, quality int) *Sink {
	s := Sink{
		w:       w,
		bitRate: bitRate,
		quality: quality,
	}
	return &s
}

// Flush cleans up buffers.
func (s *Sink) Flush(string) error {
	return s.e.Close()
}

// Sink writes buffer into file.
func (s *Sink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.e = lame.NewWriter(s.w)
	s.e.Encoder.SetBitrate(s.bitRate)
	s.e.Encoder.SetQuality(s.quality)
	s.e.Encoder.SetNumChannels(int(numChannels))
	s.e.Encoder.SetInSamplerate(int(sampleRate))
	s.e.Encoder.SetMode(lame.JOINT_STEREO)
	s.e.Encoder.SetVBR(lame.VBR_RH)
	s.e.Encoder.InitParams()

	return func(b [][]float64) error {
		buf := new(bytes.Buffer)
		ints := signal.Float64(b).AsInterInt(signal.BitDepth16, false)
		for i := range ints {
			if err := binary.Write(buf, binary.LittleEndian, int16(ints[i])); err != nil {
				return err
			}
		}
		if _, err := s.e.Write(buf.Bytes()); err != nil {
			return err
		}

		return nil
	}, nil
}
