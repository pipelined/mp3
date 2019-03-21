package mp3

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/viert/lame"

	"github.com/pipelined/signal"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// DefaultQuality for mp3 encoding.
const DefaultQuality = 5

// ChannelMode determines how mp3 file will be encoded.
type ChannelMode int

const (
	// Mono forcibly generates a mono file. If the input file is a stereo file,
	// the input stream will be read as a mono by averaging the left and right channels.
	Mono = ChannelMode(iota)
	// Stereo makes no use of potential similarity between the two input channels.
	// It can, however, negotiate the bit demand between both channels, i.e. give
	// one channel more bits if the other contains silence.
	Stereo
	// JointStereo make use of a correlation between both channels. The signal
	// will be matrixed into a sum ("mid") and difference ("side") signal. For quasi-mono
	// signals, this will give a significant gain in encoding quality. This mode does
	// not destroy phase information like IS stereo that may be used by other encoders.
	JointStereo
)

// BitRateMode determines which VBR setting is going to be used.
type BitRateMode int

const (
	// CBR uses constant bit rate.
	CBR = BitRateMode(iota)
	// ABR uses average bit rate.
	ABR
	// VBR uses variable bit rate.
	VBR
)

func (b BitRateMode) String() string {
	switch b {
	case CBR:
		return "CBR"
	case VBR:
		return "VBR"
	case ABR:
		return "ABR"
	default:
		return "Unsupported"
	}
}

// Pump allows to read mp3 data.
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

// Sink allows to send data to mp3 destinations.
type Sink struct {
	w           io.Writer
	e           *lame.LameWriter
	channelMode ChannelMode
	bitRateMode BitRateMode
	bitRate     int
	quality     int
}

// NewSink creates new Sink. DefaultQuality is used.
func NewSink(w io.Writer, cMode ChannelMode, brMode BitRateMode, bitRate int) *Sink {
	s := Sink{
		w:           w,
		channelMode: cMode,
		bitRateMode: brMode,
		bitRate:     bitRate,
		quality:     DefaultQuality,
	}
	return &s
}

// SetQuality allows to override DefaultQuality.
func (s *Sink) SetQuality(q int) {
	s.quality = q
}

// Flush cleans up buffers.
func (s *Sink) Flush(string) error {
	return s.e.Close()
}

// Sink writes buffer into destination.
func (s *Sink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.e = lame.NewWriter(s.w)
	s.e.Encoder.SetInSamplerate(sampleRate)
	s.e.Encoder.SetNumChannels(numChannels)
	s.setMode()
	s.setVBR()
	s.e.Encoder.SetBitrate(s.bitRate)
	s.e.Encoder.SetVBRQuality(s.quality)
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

// setMode assigns mode to the sink.
func (s *Sink) setVBR() {
	switch s.bitRateMode {
	case CBR:
		s.e.Encoder.SetVBR(lame.VBR_OFF)
	case VBR:
		s.e.Encoder.SetVBR(lame.VBR_MTRH)
	case ABR:
		s.e.Encoder.SetVBR(lame.VBR_ABR)
	}
}

// setMode assigns mode to the sink.
func (s Sink) setMode() {
	switch s.channelMode {
	case JointStereo:
		s.e.Encoder.SetMode(lame.JOINT_STEREO)
	case Stereo:
		s.e.Encoder.SetMode(lame.STEREO)
	case Mono:
		s.e.Encoder.SetMode(lame.MONO)
	}
}
