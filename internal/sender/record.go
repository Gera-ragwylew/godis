package sender

import (
	"context"
	"fmt"
	"godis/internal/utils/pipeline"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type recordMicrophoneStage struct {
	sender *Sender
}

func (s *Sender) RecordMicrophoneStage() pipeline.TypedStage[any, []float32] {
	return &recordMicrophoneStage{sender: s}
}

func (r *recordMicrophoneStage) Process(ctx context.Context, in <-chan any) (<-chan []float32, error) {
	return r.sender.recordMicrophone(ctx)
}

func (s *Sender) recordMicrophone(ctx context.Context) (chan []float32, error) {
	buffer := make([]float32, s.FrameSize)

	defaultInputDevice, err := portaudio.DefaultInputDevice()
	if err != nil {
		return nil, fmt.Errorf("Input device error: %w", err)
	}

	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   defaultInputDevice,
			Channels: channels,
			Latency:  60 * time.Millisecond,
		},
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: len(buffer),
		Flags:           0,
	}

	var mu sync.Mutex
	out := make(chan []float32, 20)
	callback := func(in []float32) {
		mu.Lock()
		defer mu.Unlock()

		data := make([]float32, len(in))
		copy(data, in)

		select {
		case <-ctx.Done():
			return
		case out <- data:
		default:
			log.Println("Input channel full")
		}
	}

	stream, err := portaudio.OpenStream(streamParams, callback)
	if err != nil {
		return nil, fmt.Errorf("Open stream failed: %w", err)
	}

	if err := stream.Start(); err != nil {
		stream.Stop()
		return nil, fmt.Errorf("Stream start error: %w", err)
	}
	return out, nil
}
