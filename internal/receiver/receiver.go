package receiver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	worker "godis/internal"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

const (
	HeaderSize          = 4
	jitterBufferPackets = 3
)

type Receiver struct {
	worker.BaseEntity
	worker.AudioEntity
	udpBufferPool sync.Pool
}

type PacketData struct {
	Addr     string
	Sequence uint32
	Audio    []byte
}

func New(config interface{}) *Receiver {
	r := &Receiver{}
	v := reflect.ValueOf(config)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		fmt.Println("Receiver.New() expects structure, received %v\n", v.Kind())
		return r
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		fieldName := strings.ToLower(field.Name)
		switch fieldName {
		case "port":
			r.Port = fieldValue.String()
		case "samplerate":
			r.SampleRate = fieldValue.Float()
		case "framesize":
			r.FrameSize = int(fieldValue.Int())
		}
	}

	r.udpBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 4+r.FrameSize*2)
		},
	}

	return r
}

func (r *Receiver) Start(ctx context.Context, wg *sync.WaitGroup) error {
	lc := net.ListenConfig{}
	if !strings.HasPrefix(r.Port, ":") {
		r.Port = ":" + r.Port
	}
	pc, err := lc.ListenPacket(ctx, "udp", r.Port)
	if err != nil {
		return fmt.Errorf("Failed to listen packets: %v", err)
	}
	defer pc.Close()

	conn, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return fmt.Errorf("Invalid connection type")
	}
	defer conn.Close()

	out := make([]int16, r.FrameSize)
	stream, err := portaudio.OpenDefaultStream(0, 1, r.SampleRate, r.FrameSize, out)
	if err != nil {
		return fmt.Errorf("Stream creation error: %v\n", err)
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		return fmt.Errorf("Stream start error: %w", err)
	}
	defer stream.Stop()

	jitterBufferSize := r.FrameSize * jitterBufferPackets
	audioBuffer := newRingBuffer(jitterBufferSize)

	packetChan := make(chan *PacketData, 20)
	playbackReady := make(chan struct{}, 1)

	mixer := NewMixer(r.FrameSize)

	go r.udpReceiver(ctx, conn, packetChan)
	go r.audioProcessor(ctx, packetChan, audioBuffer, playbackReady, mixer)
	return r.playbackLoop(ctx, stream, audioBuffer, playbackReady, out, mixer)
}

func (r *Receiver) udpReceiver(ctx context.Context, conn *net.UDPConn, packetChan chan<- *PacketData) {
	defer close(packetChan)

	readTimeout := 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return

		default:
			// buffer := r.udpBufferPool.Get().([]byte) // try it sometime
			buffer := make([]byte, r.FrameSize*2+4)

			conn.SetReadDeadline(time.Now().Add(readTimeout))

			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					fmt.Println("Read timeout")
					continue
				}

				if errors.Is(err, net.ErrClosed) {
					fmt.Println("Connection closed")
					return
				}

				fmt.Println("Read error: %v", err)
				continue
			}

			if n >= HeaderSize {
				sequence := binary.LittleEndian.Uint32(buffer[:HeaderSize])
				pd := &PacketData{
					Addr:     addr.String(),
					Sequence: sequence,
					Audio:    make([]byte, n-HeaderSize),
				}
				copy(pd.Audio, buffer[HeaderSize:n])

				// r.udpBufferPool.Put(buffer)

				select {
				case packetChan <- pd:
					log.Printf("Received packet from %s, seq=%d, size=%d",
						pd.Addr, pd.Sequence, len(pd.Audio))
				case <-ctx.Done():
					// r.udpBufferPool.Put(buffer)
					return
				case <-time.After(10 * time.Millisecond):
					// r.udpBufferPool.Put(buffer)
					log.Println("Packet dropped (packet channel full)")
				}
			} else {
				// r.udpBufferPool.Put(buffer)
			}
		}
	}
}

func (r *Receiver) audioProcessor(ctx context.Context, packetChan <-chan *PacketData,
	buffer *ringBuffer, playbackReady chan<- struct{}, mixer *mixer) {

	minBufferSize := r.FrameSize

	// Таймер для периодического микширования
	mixTicker := time.NewTicker(20 * time.Millisecond) // 20ms = 50 FPS
	defer mixTicker.Stop()

	// Таймер для очистки неактивных участников
	cleanupTicker := time.NewTicker(5 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case pd, ok := <-packetChan:
			if !ok {
				return
			}

			if len(pd.Audio) >= 2 && len(pd.Audio)%2 == 0 {
				samples := r.convertBytesToSamples(pd.Audio)

				// Добавляем фрейм в микшер
				if len(samples) > 0 {
					mixer.AddFrame(pd.Addr, pd.Sequence, samples)
				}
			}
		case <-mixTicker.C:
			// Каждые 20ms микшируем и отправляем в буфер
			mixedSamples := mixer.Mix()

			if mixedSamples != nil {
				buffer.Write(mixedSamples)

				if buffer.Available() >= minBufferSize && len(playbackReady) == 0 {
					select {
					case playbackReady <- struct{}{}:
					default:
					}
				}
			}

		case <-cleanupTicker.C:
			// Очищаем неактивных участников раз в 5 секунд
			mixer.Cleanup()
		}
		// 	if len(pd.Audio) >= 2 && len(pd.Audio)%2 == 0 {
		// 		samples := r.convertBytesToSamples(pd.Audio)

		// 		buffer.Write(samples)

		// 		if buffer.Available() >= minBufferSize && len(playbackReady) == 0 {
		// 			select {
		// 			case playbackReady <- struct{}{}:
		// 			default:
		// 			}
		// 		}
		// 	}

		// 	if cap(pd.Audio) >= r.FrameSize*2 {
		// 		r.udpBufferPool.Put(pd.Audio[:cap(pd.Audio)])
		// 	}
		// }
	}
}

func (r *Receiver) playbackLoop(ctx context.Context, stream *portaudio.Stream,
	buffer *ringBuffer, playbackReady <-chan struct{}, out []int16, mixer *mixer) error {

	frameDuration := time.Duration(float64(r.FrameSize)/r.SampleRate*1000) * time.Millisecond

	var stats struct {
		underflows1 int64
		underflows2 int64
		bufferLevel int
	}

	statsTimer := time.NewTicker(2 * time.Second)
	defer statsTimer.Stop()

bufferReady:
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-playbackReady:
			break bufferReady
		case <-time.After(100 * time.Millisecond):
			if buffer.Available() >= r.FrameSize {
				break bufferReady
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Playback stopped...")
			return nil

		// case <-statsTimer.C:
		// 	log.Printf("Statistics: underflows1=%d, underflows2=%d, buffer=%d samples (%.1f ms)",
		// 		stats.underflows1,
		// 		stats.underflows2,
		// 		buffer.Available(),
		// 		float64(buffer.Available())/r.SampleRate*1000)

		default:
			if buffer.Available() < r.FrameSize {
				select {
				case <-playbackReady:
				case <-time.After(10 * time.Millisecond):
					continue
				case <-ctx.Done():
					return nil
				}
			}

			n := buffer.Read(out)
			if n < r.FrameSize {
				for i := n; i < r.FrameSize; i++ {
					out[i] = 0
				}
				stats.underflows1++
			}

			if err := stream.Write(); err != nil {
				if !strings.Contains(err.Error(), "underflow") {
					log.Printf("Playback error: %v", err)
				}
				stats.underflows2++
			}

			time.Sleep(frameDuration)
		}
	}
}

func (r *Receiver) convertBytesToSamples(data []byte) []int16 {
	if len(data) < 2 || len(data)%2 != 0 {
		return nil
	}

	samples := make([]int16, len(data)/2)

	if len(data)%4 == 0 {
		for i := 0; i < len(samples); i++ {
			samples[i] = int16(data[i*2]) | (int16(data[i*2+1]) << 8)
		}
	} else {
		for i := 0; i < len(samples); i++ {
			samples[i] = int16(data[i*2]) | (int16(data[i*2+1]) << 8)
		}
	}

	return samples
}
