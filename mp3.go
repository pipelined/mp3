package mp3

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/viert/lame"

	"github.com/pipelined/signal"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// DefaultVBRQuality for algorithm quality selection of mp3 encoding.
// This setting does not affect the filesize, but affects the speed of encoding.
const DefaultVBRQuality = 3

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
	io.Reader
	d *mp3.Decoder
}

// Pump reads buffer from mp3.
func (p *Pump) Pump(sourceID string, bufferSize int) (func() ([][]float64, error), int, int, error) {
	var err error

	p.d, err = mp3.NewDecoder(p)
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

// CBRSink allows to send data to mp3 destinations with constant bit rate.
// Audio quality varies in order to maintain constant bit rate.
type CBRSink struct {
	io.Writer
	ChannelMode
	BitRate int
	e       *lame.LameWriter
}

// Flush cleans up buffers.
func (s *CBRSink) Flush(string) error {
	return s.e.Close()
}

// Sink writes buffer into destination.
func (s *CBRSink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.e = lame.NewWriter(s)
	s.e.Encoder.SetInSamplerate(sampleRate)
	s.e.Encoder.SetNumChannels(numChannels)
	setBitRateMode(s.e, CBR)
	setChannelMode(s.e, s.ChannelMode)
	s.e.Encoder.SetBitrate(s.BitRate)
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
func setBitRateMode(e *lame.LameWriter, br BitRateMode) {
	switch br {
	case CBR:
		e.Encoder.SetVBR(lame.VBR_OFF)
	case VBR:
		e.Encoder.SetVBR(lame.VBR_MTRH)
	case ABR:
		e.Encoder.SetVBR(lame.VBR_ABR)
	}
}

// setMode assigns mode to the sink.
func setChannelMode(e *lame.LameWriter, cm ChannelMode) {
	switch cm {
	case JointStereo:
		e.Encoder.SetMode(lame.JOINT_STEREO)
	case Stereo:
		e.Encoder.SetMode(lame.STEREO)
	case Mono:
		e.Encoder.SetMode(lame.MONO)
	}
}
