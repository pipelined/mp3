// Package mp3 provides pipe components that allow to read/write signal encoded in mp3 format.
package mp3

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/viert/lame"

	"github.com/pipelined/signal"
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
		fmt.Stringer
	}

	// VBR uses variable bit rate. Values: [0..10]
	VBR int

	// ABR uses average bit rate. Values: [8..320]
	ABR int

	// CBR uses constant bit rate. Values: [8..320]
	CBR int
)

// Pump allows to read mp3 data.
// This component cannot be reused for consequent runs.
type Pump struct {
	io.Reader
	d *mp3.Decoder
}

// Pump reads buffer from mp3.
func (p *Pump) Pump(sourceID string) (func(signal.Float64) error, signal.SampleRate, int, error) {
	d, err := mp3.NewDecoder(p)
	if err != nil {
		return nil, 0, 0, err
	}
	p.d = d

	// current decoder always provides stereo, so constant.
	numChannels := 2

	// buffer for output mapping.
	ints := signal.InterInt{
		NumChannels: numChannels,
		BitDepth:    signal.BitDepth16,
	}
	// current size of the buffer.
	var size int
	return func(b signal.Float64) error {
		// reset buffer size if needed.
		if b.Size() != size {
			size = b.Size()
			ints.Data = make([]int, b.Size()*numChannels)
		}

		var (
			read int
			val  int16
		)
		for read < len(ints.Data) {
			// read next frame
			if err := binary.Read(p.d, binary.LittleEndian, &val); err != nil {
				if err == io.EOF {
					break // no more bytes available
				}
				return fmt.Errorf("failed to read mp3 data: %w", err)
			}
			ints.Data[read] = int(val)
			read++
		}

		// nothing was read.
		if read == 0 {
			return io.EOF
		}
		// trim buffers.
		if read != len(ints.Data) {
			ints.Data = ints.Data[:read]
			for i := range b {
				b[i] = b[i][:ints.Size()]
			}
		}

		// convert the buffer.
		ints.CopyToFloat64(b)
		return nil
	}, signal.SampleRate(p.d.SampleRate()), numChannels, nil
}

// Sink allows to write mp3 files.
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
// Quality determines encoding algorithm quality. It doesn't affect file size.
// Use [0-9] values. It is strictly optional. Default 5 is used if no value provided.
func (s *Sink) SetQuality(q int) {
	s.quality = &q
}

// Sink writes buffer into destination.
func (s *Sink) Sink(sourceID string, sampleRate signal.SampleRate, numChannels int) (func(signal.Float64) error, error) {
	s.w = lame.NewWriter(s)
	s.BitRateMode.apply(s.w)

	if s.quality != nil {
		q := *s.quality
		s.w.Encoder.SetQuality(int(q))
	}
	setChannelMode(s.w, s.ChannelMode)
	s.w.Encoder.SetInSamplerate(int(sampleRate))
	s.w.Encoder.SetNumChannels(numChannels)
	s.w.Encoder.InitParams()
	ints := signal.InterInt{
		BitDepth:    signal.BitDepth16,
		NumChannels: numChannels,
	}
	var buf *bytes.Buffer
	return func(b signal.Float64) error {
		if b.Size() != ints.Size() {
			ints.Data = make([]int, b.Size()*numChannels)
			buf = bytes.NewBuffer(make([]byte, 0, len(ints.Data)*2))
		}
		buf.Reset()
		b.CopyToInterInt(ints)
		for _, v := range ints.Data {
			if err := binary.Write(buf, binary.LittleEndian, int16(v)); err != nil {
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
	w.Encoder.SetVBRQuality(int(vbr))
}

func (vbr VBR) String() string {
	return fmt.Sprintf("VBR(%d)", vbr)
}

func (abr ABR) apply(w *lame.LameWriter) {
	w.Encoder.SetVBR(lame.VBR_ABR)
	w.Encoder.SetVBRAverageBitRate(int(abr))
}

func (abr ABR) String() string {
	return fmt.Sprintf("ABR(%d)", abr)
}

func (cbr CBR) apply(w *lame.LameWriter) {
	w.Encoder.SetVBR(lame.VBR_OFF)
	w.Encoder.SetBitrate(int(cbr))
}

func (cbr CBR) String() string {
	return fmt.Sprintf("CBR(%d)", cbr)
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

func (cm ChannelMode) String() string {
	switch cm {
	case Mono:
		return "Mono"
	case Stereo:
		return "Stereo"
	case JointStereo:
		return "Joint Stereo"
	}
	return "Unknown"
}
