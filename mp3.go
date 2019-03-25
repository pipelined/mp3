package mp3

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/viert/lame"

	"github.com/pipelined/pipe"
	"github.com/pipelined/signal"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// ChannelMode determines how channel data will be encoded.
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

// VBRQuality determines VBR quality level. Use VBR{0-9} constants for values.
type VBRQuality int

const (
	// VBR0 results in 220 – 260 Kbps.
	VBR0 = VBRQuality(iota)
	// VBR1 results in 190 – 250 Kbps.
	VBR1
	// VBR2 results in 170 – 210 Kbps.
	VBR2
	// VBR3 results in 150 – 195 Kbps.
	VBR3
	// VBR4 results in 140 – 185 Kbps.
	VBR4
	// VBR5 results in 120 – 150 Kbps.
	VBR5
	// VBR6 results in 100 – 130 Kbps.
	VBR6
	// VBR7 results in 80 - 110 Kbps.
	VBR7
	// VBR8 results in 70 - 95 Kbps.
	VBR8
	// VBR9 results in 60 - 80 Kbps.
	VBR9
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

// Quality determines encoding algorithm quality. It doesn't affect file size.
// Use Q{0-9} constants for function calls. It is strictly optional.
type Quality int

const (
	// Q0 sets maximum quality algorithm selection.
	Q0 = Quality(iota)
	// Q1 sets quality to 1.
	Q1
	// Q2 sets quality to 2.
	Q2
	// Q3 sets quality to 3. This is the value used by default.
	Q3
	// Q4 sets quality to 4.
	Q4
	// Q5 sets quality to 5.
	Q5
	// Q6 sets quality to 6.
	Q6
	// Q7 sets quality to 7.
	Q7
	// Q8 sets quality to 8.
	Q8
	// Q9 sets minimum quality algorithm selection.
	Q9
)

// Sink is a generic mp3 sink interface. It is implemented by:
//
//		VBRSink: encodes with variable bit rate
//		CBRSink: encodes with constant bit rate
//		ABRSink: encodes with average bit rate
//
// They also have different encoding parameters.
type Sink interface {
	pipe.Sink
	pipe.Flusher
	SetQuality(Quality)
}

// sink wraps LameWriter and contains generic logic.
type sink struct {
	quality *Quality
	w       *lame.LameWriter
}

// Flush cleans up buffers.
func (s *sink) Flush(string) error {
	return s.w.Close()
}

// SetQuality sets the quality to the lame encoder.
// Q3 is used if you don't call this method.
func (s *sink) SetQuality(q Quality) {
	s.quality = &q
}

// CBRSink allows to send data to mp3 destinations with constant bit rate.
// Audio quality varies in order to maintain constant bit rate.
type CBRSink struct {
	io.Writer
	ChannelMode
	BitRate int
	sink
}

// Sink writes buffer into destination.
func (s *CBRSink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.w = lame.NewWriter(s)
	s.w.Encoder.SetBitrate(s.BitRate)

	return sinkFn(s.sink, CBR, s.ChannelMode, sampleRate, numChannels), nil
}

// ABRSink allows to send data to mp3 destinations with averaged bit rate.
// Audio quality and bit rate both vary. A cross between VBR and CBR.
type ABRSink struct {
	io.Writer
	ChannelMode
	BitRate int
	sink
}

// Sink writes buffer into destination.
func (s *ABRSink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.w = lame.NewWriter(s)
	s.w.Encoder.SetVBRAverageBitRate(s.BitRate)

	return sinkFn(s.sink, ABR, s.ChannelMode, sampleRate, numChannels), nil
}

// VBRSink allows to send data to mp3 destinations with varied bit rate.
// Bit rate varies in order to maintain constant audio quality.
type VBRSink struct {
	io.Writer
	ChannelMode
	VBRQuality
	sink
}

// Sink writes buffer into destination.
func (s *VBRSink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.w = lame.NewWriter(s)
	s.w.Encoder.SetVBRQuality(int(s.VBRQuality))
	return sinkFn(s.sink, VBR, s.ChannelMode, sampleRate, numChannels), nil
}

// sinkFn is a generic sink closure for lame writer.
func sinkFn(s sink, bitRateMode BitRateMode, channelMode ChannelMode, sampleRate, numChannels int) func([][]float64) error {
	if s.quality != nil {
		q := *s.quality
		s.w.Encoder.SetQuality(int(q))
	}
	setBitRateMode(s.w, bitRateMode)
	setChannelMode(s.w, channelMode)
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
	}
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
