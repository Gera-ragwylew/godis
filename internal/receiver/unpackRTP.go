package receiver

import (
	"context"
	"fmt"
	"godis/internal/utils/jitterbuffer"
	"godis/internal/utils/pipeline"
	rtputils "godis/internal/utils/rtp"
	"log"

	"github.com/hraban/opus"
)

type UnpackRTPStage struct {
	receiver *Receiver
}

func (r *Receiver) UnpackRTPStage() pipeline.TypedStage[[]byte, *rtputils.RTPPacket] {
	return &UnpackRTPStage{receiver: r}
}

func (c *UnpackRTPStage) Process(ctx context.Context, in <-chan []byte) (<-chan *rtputils.RTPPacket, error) {
	return c.receiver.unpackRTP(ctx, in)
}

func (r *Receiver) unpackRTP(ctx context.Context, in <-chan []byte) (<-chan *rtputils.RTPPacket, error) {
	out := make(chan *rtputils.RTPPacket, 20)

	depacketizer := rtputils.NewOpusDepacketizer(rtputils.DefaultOpusConfig().PayloadType)

	jb := jitterbuffer.NewJitterBuffer(jitterbuffer.DefaultJitterbufferConfig())
	r.audioBuffer = jb

	decoder, err := opus.NewDecoder(int(r.SampleRate), 1)
	if err != nil {
		return nil, fmt.Errorf("Failed to create decoder: %v", err)
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-in:
				if !ok {
					return
				}

				RTPpacket, err := depacketizer.Depacketize(data)
				if err != nil {
					log.Println("Depacketize error: ", err)
				}

				PCMbuffer := make([]int16, r.FrameSize)
				decodedSamples, err := decoder.Decode(RTPpacket.Payload, PCMbuffer)
				if err != nil {
					log.Println("Decode error: ", err)
					continue
				}

				if decodedSamples > 0 {
					pcm := make([]int16, decodedSamples)
					copy(pcm, PCMbuffer[:decodedSamples])
					RTPpacket.PCMData = pcm

					jb.AddPacket(RTPpacket)
				}

			}
		}
	}()
	return out, nil
}

// func packetLogger(streamStats map[uint32]uint16, ssrc uint32, seq uint16) {
// 	var lostCount uint32

// 	_, exists := streamStats[ssrc]
// 	if !exists {
// 		streamStats[ssrc] = seq
// 		return
// 	}

// 	expected := streamStats[ssrc] + 1

// 	diff := int32(seq) - int32(expected)
// 	if diff < -32768 {
// 		diff += 65536
// 	} else if diff > 32767 {
// 		diff -= 65536
// 	}

// 	if diff > 0 {
// 		lostCount += uint32(diff)
// 		log.Printf("Lost %d packets: SSRC=%d, seq %d -> %d",
// 			diff, ssrc, expected, seq)
// 	} else if diff < 0 {
// 		log.Printf("Out of order packet: SSRC=%d, seq %d (expected %d)",
// 			ssrc, seq, expected)
// 	}

// 	streamStats[ssrc] = seq
// }
