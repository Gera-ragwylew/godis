package receiver

import (
	"context"
	"errors"
	"fmt"
	"godis/internal/pipeline"
	"log"
	"net"
	"strings"
	"time"
)

type receiveUDPStage struct {
	receiver *Receiver
}

func (r *Receiver) ReceiveUDPStage() pipeline.TypedStage[any, *PacketData] {
	return &receiveUDPStage{receiver: r}
}

func (r *receiveUDPStage) Process(ctx context.Context, in <-chan any) (<-chan *PacketData, error) {
	return r.receiver.receiveUDP(ctx, in)
}

func (r *Receiver) receiveUDP(ctx context.Context, in <-chan any) (<-chan *PacketData, error) {
	out := make(chan *PacketData, 20)
	lc := net.ListenConfig{}
	if !strings.HasPrefix(r.Port, ":") {
		r.Port = ":" + r.Port
	}
	pc, err := lc.ListenPacket(ctx, "udp", r.Port)
	if err != nil {
		return nil, fmt.Errorf("Failed to listen packets: %v", err)
	}

	conn, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return nil, fmt.Errorf("Invalid connection type")
	}

	readTimeout := 100 * time.Millisecond

	go func() {
		defer func() {
			if conn != nil {
				conn.Close()
			}
			close(out)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn.SetReadDeadline(time.Now().Add(readTimeout))

				buffer := make([]byte, r.FrameSize)
				n, addr, err := conn.ReadFromUDP(buffer)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Println("Read timeout")
						continue
					}

					if errors.Is(err, net.ErrClosed) {
						log.Println("Connection closed")
						return
					}

					log.Println("Read error: ", err)
					continue
				}

				if n >= 0 {
					pd := &PacketData{
						Addr:     addr.String(),
						Sequence: 0,
						Audio:    make([]byte, n),
					}
					copy(pd.Audio, buffer[:n])

					select {
					case out <- pd:
						log.Printf("Received packet from %s, seq=%d, size=%d",
							pd.Addr, pd.Sequence, len(pd.Audio))
					case <-ctx.Done():
						return
					case <-time.After(10 * time.Millisecond):
						log.Println("Packet dropped (packet channel full)")
					}
				}
			}
		}
	}()
	return out, nil
}
