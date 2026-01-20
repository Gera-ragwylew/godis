package receiver

import (
	"context"
	"fmt"
	"godis/internal/pipeline"
	"log"
	"time"

	"github.com/gordonklaus/portaudio"
)

type playbackStage struct {
	receiver *Receiver
}

func (r *Receiver) PlaybackStage() pipeline.TypedStage[struct{}, any] {
	return &playbackStage{receiver: r}
}

func (r *playbackStage) Process(ctx context.Context, in <-chan struct{}) (<-chan any, error) {
	return r.receiver.playback(ctx, in)
}

func (r *Receiver) playback(ctx context.Context, _ <-chan struct{}) (<-chan any, error) {
	outBuffer := make([]int16, r.FrameSize)
	stream, err := portaudio.OpenDefaultStream(0, 1, r.SampleRate, r.FrameSize, &outBuffer)
	if err != nil {
		return nil, fmt.Errorf("Stream creation error: %v\n", err)
	}

	if err := stream.Start(); err != nil {
		stream.Stop()
		stream.Close()
		return nil, fmt.Errorf("Stream start error: %w", err)
	}

	frameDuration := time.Duration(float64(r.FrameSize)/r.SampleRate*1000) * time.Millisecond
	playbackTicker := time.NewTicker(frameDuration)

	go func() {
		defer func() {
			stream.Stop()
			stream.Close()
			playbackTicker.Stop()
		}()

		for {
			select {
			case <-ctx.Done():
				log.Println("Playback stopped...")
				return
			case <-playbackTicker.C:
				if r.audioBuffer.Available() >= 5 {
					buffer := make([]*opusData, 20)
					n := r.audioBuffer.Read(buffer)
					if n > 0 {
						for i := range n {
							copy(outBuffer, buffer[i].Audio)
							if err := stream.Write(); err != nil {
								log.Println("Stream write error: ", err)
							}
						}

					}
				}
			}
		}
	}()
	return nil, nil
}
