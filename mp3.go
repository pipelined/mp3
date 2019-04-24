package mp3

//go:generate stringer -type=ChannelMode -output=stringers.go

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/viert/lame"

	"github.com/pipelined/signal"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// ChannelMode determines how channel data will be encoded.
type ChannelMode int

const (
	// Mono forcibly generates a mono file. If the input file is a stereo file,
	// the input stream will be read as a mono by averaging the left and right channels.
	Mono ChannelMode = iota
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

type (
	// BitRateMode determines which VBR setting is going to be used.
	BitRateMode interface {
		apply(*lame.LameWriter)
	}

	// VBR uses variable bit rate.
	VBR struct {
		Quality int
	}

	// ABR uses average bit rate.
	ABR struct {
		BitRate int
	}

	// CBR uses constant bit rate.
	CBR struct {
		BitRate int
	}
)

const (
	// MinBitRate is the minimal bit rate value that could be used for CBR/ABR sinks.
	MinBitRate = 8
	// MaxBitRate is the maximal bit rate value that could be used for CBR/ABR sinks.
	MaxBitRate = 320
	// DefaultExtension of mp3 files.
	DefaultExtension = ".mp3"
)

var (
	// Supported values for convert configuration.
	Supported = supported{
		channelModes: map[ChannelMode]struct{}{
			JointStereo: {},
			Stereo:      {},
			Mono:        {},
		},
	}
	// extensions of mp3 files.
	extensions = []string{
		DefaultExtension,
	}
)

type supported struct {
	bitRateModes map[BitRateMode]struct{}
	channelModes map[ChannelMode]struct{}
}

// Extensions of mp3 files.
func Extensions() []string {
	return extensions
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

// Sink is a generic mp3 sink interface. It is implemented by:
//
//		VBRSink: encodes with variable bit rate
//		CBRSink: encodes with constant bit rate
//		ABRSink: encodes with average bit rate
//
// They also have different encoding parameters.
// Quality determines encoding algorithm quality. It doesn't affect file size.
// Use [0-9] values. It is strictly optional.
// type Quality int
type Sink struct {
	io.Writer
	BitRateMode
	ChannelMode
	quality *int
	w       *lame.LameWriter
}

// Flush cleans up buffers.
func (s *Sink) Flush(string) error {
	return s.w.Close()
}

// SetQuality sets the quality to the lame encoder.
// Q3 is used if you don't call this method.
func (s *Sink) SetQuality(q int) {
	s.quality = &q
}

// Sink writes buffer into destination.
func (s *Sink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.w = lame.NewWriter(s)
	s.BitRateMode.apply(s.w)

	if s.quality != nil {
		q := *s.quality
		s.w.Encoder.SetQuality(int(q))
	}
	setChannelMode(s.w, s.ChannelMode)
	s.w.Encoder.SetInSamplerate(sampleRate)
	s.w.Encoder.SetNumChannels(numChannels)
	s.w.Encoder.InitParams()
	return func(b [][]float64) error {
		buf := new(bytes.Buffer)
		ints := signal.Float64(b).AsInterInt(signal.BitDepth16, false)
		for i := range ints {
			if err := binary.Write(buf, binary.LittleEndian, int16(ints[i])); err != nil {
				return err
			}
		}
		if _, err := s.w.Write(buf.Bytes()); err != nil {
			return err
		}
		return nil
	}, nil
}

func (vbr VBR) apply(w *lame.LameWriter) {
	w.Encoder.SetVBR(lame.VBR_MTRH)
	w.Encoder.SetVBRQuality(vbr.Quality)
}

func (abr ABR) apply(w *lame.LameWriter) {
	w.Encoder.SetVBR(lame.VBR_ABR)
	w.Encoder.SetVBRAverageBitRate(abr.BitRate)
}

func (cbr CBR) apply(w *lame.LameWriter) {
	w.Encoder.SetVBR(lame.VBR_OFF)
	w.Encoder.SetBitrate(cbr.BitRate)
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

// ChannelMode checks if provided channel mode is supported.
func (s supported) ChannelMode(v ChannelMode) error {
	if _, ok := s.channelModes[v]; !ok {
		return fmt.Errorf("Channel mode %v is not supported", v)
	}
	return nil
}

// Quality checks if provided quality is supported.
func (s supported) Quality(v int) error {
	if v < 0 || v > 9 {
		return fmt.Errorf("Quality %v is not supported. Provide value between 0 and 9", v)
	}
	return nil
}

// BitRateMode checks if provided bit rate mode is supported.
// It also validates if provided mode has valid settings:
// 	* VBR quality for VBR;
//	* Bit rate for ABR and CBR.
func (s supported) BitRateMode(v BitRateMode) error {
	switch t := v.(type) {
	case VBR:
		return Supported.vbrQuality(t.Quality)
	case CBR:
		return Supported.bitRate(t.BitRate)
	case ABR:
		return Supported.bitRate(t.BitRate)
	default:
		return fmt.Errorf("Bit rate mode %T is not supported", t)
	}
}

// VBRQuality checks if provided VBR quality is supported.
func (s supported) vbrQuality(v int) error {
	if v < 0 || v > 9 {
		return fmt.Errorf("VBR quality %v is not supported. Provide value between 0 and 9", v)
	}
	return nil
}

// BitRate checks if provided bit rate is supported.
func (s supported) bitRate(v int) error {
	if v > MaxBitRate || v < MinBitRate {
		return fmt.Errorf("Bit rate %v is not supported. Provide value between %d and %d", v, MinBitRate, MaxBitRate)
	}
	return nil
}

func (s supported) ChannelModes() map[ChannelMode]struct{} {
	result := make(map[ChannelMode]struct{})
	for k, v := range s.channelModes {
		result[k] = v
	}
	return result
}
