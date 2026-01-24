package rtputils

import (
	"fmt"
	"log"
	"time"

	"github.com/pion/rtp"
)

const (
	rtpHeaderSize = 12
)

type RTPPacket struct {
	SSRC       uint32
	Sequence   uint16
	Timestamp  uint32
	ReceivedAt time.Time

	Payload []byte
	PCMData []int16

	PayloadType uint8
	Marker      bool
}

type RTPConfig struct {
	PayloadType uint8
	SSRC        uint32
	ClockRate   uint32
	Mtu         uint16
}

func DefaultOpusConfig() RTPConfig {
	return RTPConfig{
		PayloadType: 96,
		ClockRate:   48000,
		SSRC:        GenerateSSRC(),
		Mtu:         1200,
	}
}

func GenerateSSRC() uint32 {
	return uint32(time.Now().UnixNano() & 0xFFFFFFFF)
}

type OpusPayloader struct{}

func (p *OpusPayloader) Payload(mtu uint16, payload []byte) [][]byte {
	if len(payload) == 0 {
		return [][]byte{}
	}

	maxPayloadSize := int(mtu) - rtpHeaderSize

	if len(payload) <= maxPayloadSize {
		return [][]byte{payload}
	}

	var payloads [][]byte
	for len(payload) > maxPayloadSize {
		payloads = append(payloads, payload[:maxPayloadSize])
		payload = payload[maxPayloadSize:]
	}

	if len(payload) > 0 {
		payloads = append(payloads, payload)
	}

	return payloads
}

type Packetizer struct {
	packetizer rtp.Packetizer
}

func NewOpusPacketizer(config RTPConfig) *Packetizer {
	packetizer := rtp.NewPacketizer(
		config.Mtu,
		config.PayloadType,
		config.SSRC,
		&OpusPayloader{},
		rtp.NewRandomSequencer(),
		config.ClockRate,
	)

	return &Packetizer{
		packetizer: packetizer,
	}
}

func (p *Packetizer) Packetize(opusData []byte, samples int) ([][]byte, error) {
	if len(opusData) == 0 {
		return nil, fmt.Errorf("opus data is empty")
	}

	packets := p.packetizer.Packetize(opusData, uint32(samples))
	if len(packets) > 1 {
		log.Println("Packetize more then one packet")
	}

	result := make([][]byte, 0, len(packets))

	for _, packet := range packets {
		data, err := packet.Marshal()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal RTP packet: %w", err)
		}
		result = append(result, data)
	}

	return result, nil
}

type Depacketizer struct {
}

func (d Depacketizer) Unmarshal(packet []byte) ([]byte, error) {
	if len(packet) == 0 {
		return nil, fmt.Errorf("empty opus packet")
	}

	toc := packet[0]

	// Configuration number: bits 0-4
	config := (toc >> 3) & 0x1F

	if config > 18 {
		return nil, fmt.Errorf("invalid opus configuration: %d", config)
	}

	// Stereo flag: bit 2
	// stereo := (toc >> 2) & 0x01

	// Frame count: bits 0-1 for CBR, and other for VBR
	// frameCount := toc & 0x03

	return packet, nil
}

// These functions only implement the interface and are not used directly
func (d Depacketizer) IsPartitionHead(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	return true
}

func (d Depacketizer) IsPartitionTail(marker bool, payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	toc := payload[0]

	frameCount := toc & 0x03

	return marker || frameCount == 0
}

type RTPDepacketizer struct {
	expectedPayloadType uint8
	depacketizer        rtp.Depacketizer
	lastSequence        uint16
	stats               ReceptionStats
}

type ReceptionStats struct {
	PacketsReceived uint32
	BytesReceived   uint64
	PacketsLost     uint32
	BadPackets      uint32
	FirstReceivedAt time.Time
	LastReceivedAt  time.Time
}

func NewOpusDepacketizer(expectedPayloadType uint8) *RTPDepacketizer {
	return &RTPDepacketizer{
		expectedPayloadType: expectedPayloadType,
		depacketizer:        Depacketizer{},
		lastSequence:        0,
		stats: ReceptionStats{
			FirstReceivedAt: time.Now(),
		},
	}
}

func (d *RTPDepacketizer) Depacketize(rtpData []byte) (*RTPPacket, error) {
	if len(rtpData) < 12 {
		d.stats.BadPackets++
		return nil, fmt.Errorf("RTP packet too short: %d bytes", len(rtpData))
	}

	packet := &rtp.Packet{}
	if err := packet.Unmarshal(rtpData); err != nil {
		d.stats.BadPackets++
		return nil, fmt.Errorf("failed to unmarshal RTP packet: %w", err)
	}

	if packet.Header.Version != 2 {
		d.stats.BadPackets++
		return nil, fmt.Errorf("unsupported RTP version: %d", packet.Header.Version)
	}

	if d.expectedPayloadType != 0 && packet.Header.PayloadType != d.expectedPayloadType {
		d.stats.BadPackets++
		return nil, fmt.Errorf("unexpected payload type: got %d, expected %d",
			packet.Header.PayloadType, d.expectedPayloadType)
	}

	if d.stats.PacketsReceived > 0 {
		expected := d.lastSequence + 1
		if packet.Header.SequenceNumber != expected {
			lost := uint16(packet.Header.SequenceNumber - d.lastSequence - 1)
			d.stats.PacketsLost += uint32(lost)

			if lost > 0 {
				log.Printf("Detected %d lost packets. Last: %d, Current: %d\n",
					lost, d.lastSequence, packet.Header.SequenceNumber)
			}
		}
	}

	d.stats.PacketsReceived++
	d.stats.BytesReceived += uint64(len(rtpData))
	d.lastSequence = packet.Header.SequenceNumber
	d.stats.LastReceivedAt = time.Now()

	result := &RTPPacket{
		SSRC:       packet.Header.SSRC,
		Sequence:   packet.Header.SequenceNumber,
		Timestamp:  packet.Header.Timestamp,
		ReceivedAt: time.Now(),

		Payload: packet.Payload,

		PayloadType: packet.Header.PayloadType,
		Marker:      packet.Header.Marker,
	}

	return result, nil
}

func (d *RTPDepacketizer) GetStats() ReceptionStats {
	return d.stats
}

func (d *RTPDepacketizer) ResetStats() {
	d.stats = ReceptionStats{
		FirstReceivedAt: time.Now(),
	}
	d.lastSequence = 0
}
