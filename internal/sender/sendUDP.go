package sender

import (
	"context"
	"fmt"
	"godis/internal/utils/pipeline"
	"log"
	"net"
)

type sendUDPStage struct {
	sender *Sender
}

func (s *Sender) SendUDPStage() pipeline.TypedStage[[]byte, any] {
	return &sendUDPStage{sender: s}
}

func (r *sendUDPStage) Process(ctx context.Context, in <-chan []byte) (<-chan any, error) {
	return r.sender.sendUDP(ctx, in)
}

func (s *Sender) sendUDP(ctx context.Context, in <-chan []byte) (<-chan any, error) {
	var conns []net.Conn
	for _, addr := range s.Ip {
		conn, err := net.Dial("udp", addr+":"+s.Port)
		if err != nil {
			for _, c := range conns {
				c.Close()
			}
			return nil, fmt.Errorf("UDP dial error to %s: %w", addr, err)
		}
		conns = append(conns, conn)
	}
	go func() {
		defer func() {
			for _, conn := range conns {
				conn.Close()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case packet, ok := <-in:
				if !ok {
					return
				}
				for _, conn := range conns {
					if _, err := conn.Write(packet); err != nil {
						log.Println("Write packet error: ", err)
					}
				}
			}
		}
	}()
	return nil, nil
}
