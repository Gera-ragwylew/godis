package sender

import (
	"context"
	"godis/internal/utils/pipeline"
	rtputils "godis/internal/utils/rtp"
	"log"
)

type packAsRTPStage struct {
	sender *Sender
}

func (s *Sender) PackAsRTPStage() pipeline.TypedStage[[]byte, []byte] {
	return &packAsRTPStage{sender: s}
}

func (c *packAsRTPStage) Process(ctx context.Context, in <-chan []byte) (<-chan []byte, error) {
	return c.sender.packAsRTP(ctx, in)
}

func (s *Sender) packAsRTP(ctx context.Context, in <-chan []byte) (chan []byte, error) {
	out := make(chan []byte, 20)
	p := rtputils.NewOpusPacketizer(rtputils.DefaultOpusConfig())
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

				RTPpackets, err := p.Packetize(data, s.FrameSize)
				if err != nil {
					log.Println("Packetize error: ", err)
				}

				for _, packet := range RTPpackets {
					select {
					case <-ctx.Done():
						return
					case out <- packet:
					}
				}

			}
		}
	}()
	return out, nil
}

// func (p *OpusPacketizer) PacketizeSingle(opusData []byte, samples int) ([]byte, error) {
// 	p.timestamp += uint32(samples)

// 	packet := &rtp.Packet{
// 		Header: rtp.Header{
// 			Version:        2,
// 			Padding:        false,
// 			Extension:      false,
// 			Marker:         false,
// 			PayloadType:    p.config.PayloadType,
// 			SequenceNumber: p.sequencer.NextSequenceNumber(),
// 			Timestamp:      p.timestamp,
// 			SSRC:           p.config.SSRC,
// 		},
// 		Payload: opusData,
// 	}

// 	data, err := packet.Marshal()
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to marshal RTP packet: %w", err)
// 	}

// 	if len(data) > int(p.config.MaxPacketSize) {
// 		return nil, fmt.Errorf("packet size %d exceeds MTU %d",
// 			len(data), p.config.MaxPacketSize)
// 	}

// 	return data, nil
// }
