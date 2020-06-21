// Package mp3 provides pipe components that allow to read/write signal
// encoded in mp3 format.
package mp3

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/viert/lame"

	"pipelined.dev/pipe"
	"pipelined.dev/signal"
)

// ChannelMode determines how channel data will be encoded.
type ChannelMode int

const (
	// Mono forcibly generates a mono file. If the input file is a stereo
	// file, the input stream will be read as a mono by averaging the left
	// and right channels.
	Mono ChannelMode = iota
	// Stereo makes no use of potential similarity between the two input
	// channels. It can, however, negotiate the bit demand between both
	// channels, i.e. give one channel more bits if the other contains
	// silence.
	Stereo
	// JointStereo make use of a correlation between both channels. The
	// signal will be matrixed into a sum ("mid") and difference ("side")
	// signal. For quasi-mono signals, this will give a significant gain in
	// encoding quality. This mode does not destroy phase information like
	// IS stereo that may be used by other encoders.
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
	decoder *mp3.Decoder
}

// Pump reads buffer from mp3.
func (p *Pump) Pump() pipe.SourceAllocatorFunc {
	return func(bufferSize int) (pipe.Source, pipe.SignalProperties, error) {
		decoder, err := mp3.NewDecoder(p)
		if err != nil {
			return pipe.Source{}, pipe.SignalProperties{}, fmt.Errorf("error creating MP3 decoder: %w", err)
		}
		p.decoder = decoder

		// current decoder always provides stereo, so constant.
		channels := 2
		ints := signal.Allocator{
			Channels: channels,
			Capacity: bufferSize,
			Length:   bufferSize,
		}.Int16(signal.BitDepth16)
		return pipe.Source{SourceFunc: p.source(ints)},
			pipe.SignalProperties{
				Channels:   channels,
				SampleRate: signal.SampleRate(p.decoder.SampleRate()),
			},
			nil
	}
}

func (p *Pump) source(ints signal.Signed) pipe.SourceFunc {
	return func(floats signal.Floating) (int, error) {
		var read int // total number of read samples
		for read < ints.Len() {
			var sample int16
			if err := binary.Read(p.decoder, binary.LittleEndian, &sample); err != nil {
				// because EOF returns only when nothing was read.
				if err == io.EOF {
					break // no more bytes available
				}
				return read, fmt.Errorf("error reading MP3 data: %w", err)
			}
			ints.SetSample(read, int64(sample))
			read++
		}

		// nothing was read, source is done.
		if read == 0 {
			return 0, io.EOF
		}
		if read != ints.Len() {
			return signal.SignedAsFloating(ints.Slice(0, signal.ChannelLength(read, ints.Channels())), floats), nil
		}
		return signal.SignedAsFloating(ints, floats), nil
	}
}

// Sink allows to write mp3 files.
type Sink struct {
	io.Writer
	BitRateMode
	ChannelMode
	quality *int
	writer  *lame.LameWriter
}

// Flush cleans up buffers.
func (s *Sink) Flush(context.Context) error {
	return s.writer.Close()
}

// SetQuality sets the quality to the lame encoder. Quality determines
// encoding algorithm quality. It doesn't affect file size. Use [0-9]
// values. It is strictly optional. Default 5 is used if no value provided.
func (s *Sink) SetQuality(q int) {
	s.quality = &q
}

// Sink writes buffer into destination.
func (s *Sink) Sink() pipe.SinkAllocatorFunc {
	return func(bufferSize int, props pipe.SignalProperties) (pipe.Sink, error) {
		s.writer = lame.NewWriter(s)
		s.BitRateMode.apply(s.writer)

		if s.quality != nil {
			s.writer.Encoder.SetQuality(*s.quality)
		}
		setChannelMode(s.writer, s.ChannelMode)
		s.writer.Encoder.SetInSamplerate(int(props.SampleRate))
		s.writer.Encoder.SetNumChannels(int(props.Channels))
		s.writer.Encoder.InitParams()
		ints := signal.Allocator{
			Channels: props.Channels,
			Capacity: bufferSize,
			Length:   bufferSize,
		}.Int16(signal.BitDepth16)
		return pipe.Sink{
			SinkFunc:  s.sink(ints),
			FlushFunc: s.Flush,
		}, nil
	}
}

func (s *Sink) sink(ints signal.Signed) pipe.SinkFunc {
	bytesBuf := bytes.NewBuffer(make([]byte, 0, ints.Len()))
	return func(floats signal.Floating) error {
		if n := signal.FloatingAsSigned(floats, ints); n != ints.Length() {
			ints = ints.Slice(0, n)
			// defer because it must be done after write
			defer func() {
				ints = ints.Slice(0, ints.Capacity())
			}()
		}
		bytesBuf.Reset()
		for i := 0; i < ints.Len(); i++ {
			if err := binary.Write(bytesBuf, binary.LittleEndian, int16(ints.Sample(i))); err != nil {
				return fmt.Errorf("error writing binary data: %w", err)
			}
		}
		if _, err := s.writer.Write(bytesBuf.Bytes()); err != nil {
			return fmt.Errorf("error writing MP3 buffer: %w", err)
		}
		return nil
	}
}

func (vbr VBR) apply(writer *lame.LameWriter) {
	writer.Encoder.SetVBR(lame.VBR_MTRH)
	writer.Encoder.SetVBRQuality(int(vbr))
}

func (vbr VBR) String() string {
	return fmt.Sprintf("vbr-%d", vbr)
}

func (abr ABR) apply(writer *lame.LameWriter) {
	writer.Encoder.SetVBR(lame.VBR_ABR)
	writer.Encoder.SetVBRAverageBitRate(int(abr))
}

func (abr ABR) String() string {
	return fmt.Sprintf("abr-%d", abr)
}

func (cbr CBR) apply(writer *lame.LameWriter) {
	writer.Encoder.SetVBR(lame.VBR_OFF)
	writer.Encoder.SetBitrate(int(cbr))
}

func (cbr CBR) String() string {
	return fmt.Sprintf("cbr-%d", cbr)
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
