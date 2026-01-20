package sender

import (
	"context"
	"errors"
	"fmt"
	"godis/internal/pipeline"
	"log"
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
	out := make(chan []float32, 20)
	stream, err := portaudio.OpenDefaultStream(channels, 0, s.SampleRate, s.FrameSize, buffer)
	if err != nil {
		return nil, fmt.Errorf("Stream creation error: %v\n", err)
	}

	if err := stream.Start(); err != nil {
		stream.Stop()
		return nil, fmt.Errorf("Stream start error: %w", err)
	}

	frameDuration := time.Duration(float64(s.FrameSize)/s.SampleRate*1000) * time.Millisecond // 60 ms
	ticker := time.NewTicker(frameDuration)

	go func() {
		defer func() {
			stream.Stop()
			stream.Close()
			ticker.Stop()
			close(out)
			log.Println("Recording stopped and resources cleaned up")
		}()

		for {
			select {
			case <-ctx.Done():
				log.Println("Stop recording by context...")
				return
			case <-ticker.C:
				if err := stream.Read(); err != nil {
					log.Printf("Stream reading error: %v\n", err)
					if errors.Is(err, portaudio.InputOverflowed) {
						log.Println("Error: ", err)
						return
					}
					continue
				}

				data := make([]float32, len(buffer))
				copy(data, buffer)

				select {
				case <-ctx.Done():
					return
				case out <- data:
				}
			}
		}
	}()
	return out, nil
}
