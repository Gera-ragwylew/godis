package jitterbuffer

import (
	"container/list"
	rtputils "godis/internal/utils/rtp"
	"sync"
	"time"
)

type StreamBuffer struct {
	SSRC          uint32
	packets       *list.List
	expectedSeq   uint16
	lastTimestamp uint32
	lastReadTime  time.Time
	mu            sync.RWMutex
	sampleRate    uint32
	frameSize     uint32
	maxBufferSize int
	packetsMap    map[uint16]*list.Element
}

type JitterBuffer struct {
	streams     map[uint32]*StreamBuffer
	mu          sync.RWMutex
	maxStreams  int
	sampleRate  uint32
	frameSize   uint32
	readTimeout time.Duration
	bufferTime  time.Duration
}

type Config struct {
	SampleRate  uint32
	FrameSize   uint32
	MaxStreams  int
	ReadTimeout time.Duration
	BufferTime  time.Duration
}

func DefaultJitterbufferConfig() Config {
	return Config{
		SampleRate:  16000,
		FrameSize:   960,
		MaxStreams:  5,
		ReadTimeout: 70 * time.Millisecond,
		BufferTime:  30 * time.Millisecond,
	}
}

func NewJitterBuffer(config Config) *JitterBuffer {
	if config.MaxStreams <= 0 {
		config.MaxStreams = 5
	}
	if config.BufferTime <= 0 {
		config.BufferTime = 70 * time.Millisecond
	}

	return &JitterBuffer{
		streams:     make(map[uint32]*StreamBuffer),
		maxStreams:  config.MaxStreams,
		sampleRate:  config.SampleRate,
		frameSize:   config.FrameSize,
		readTimeout: config.ReadTimeout,
		bufferTime:  config.BufferTime,
	}
}

func (jb *JitterBuffer) AddPacket(packet *rtputils.RTPPacket) {
	jb.mu.RLock()
	stream, exists := jb.streams[packet.SSRC]
	jb.mu.RUnlock()

	if !exists {
		jb.mu.Lock()
		if len(jb.streams) >= jb.maxStreams {
			jb.removeInactiveStream()
		}

		stream = jb.createStream(packet.SSRC)
		jb.streams[packet.SSRC] = stream
		jb.mu.Unlock()
	}

	stream.addPacket(packet)
}

func (jb *JitterBuffer) GetPacket(ssrc uint32) (*rtputils.RTPPacket, error) {
	jb.mu.RLock()
	stream, exists := jb.streams[ssrc]
	jb.mu.RUnlock()

	if !exists {
		return nil, nil
	}

	return stream.getPacket(jb.bufferTime)
}

func (jb *JitterBuffer) RemoveStream(ssrc uint32) {
	jb.mu.Lock()
	delete(jb.streams, ssrc)
	jb.mu.Unlock()
}

func (jb *JitterBuffer) GetActiveStreams() []uint32 {
	jb.mu.RLock()
	defer jb.mu.RUnlock()

	streams := make([]uint32, 0, len(jb.streams))
	for ssrc := range jb.streams {
		streams = append(streams, ssrc)
	}
	return streams
}

func (jb *JitterBuffer) Cleanup(timeout time.Duration) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	now := time.Now()
	for ssrc, stream := range jb.streams {
		stream.mu.RLock()
		lastRead := stream.lastReadTime
		stream.mu.RUnlock()

		if now.Sub(lastRead) > timeout {
			delete(jb.streams, ssrc)
		}
	}
}

func (jb *JitterBuffer) createStream(ssrc uint32) *StreamBuffer {
	return &StreamBuffer{
		SSRC:          ssrc,
		packets:       list.New(),
		packetsMap:    make(map[uint16]*list.Element),
		sampleRate:    jb.sampleRate,
		frameSize:     jb.frameSize,
		maxBufferSize: 100,
		expectedSeq:   0,
		lastReadTime:  time.Now(),
	}
}

func (jb *JitterBuffer) removeInactiveStream() {
	var oldestSSRC uint32
	var oldestTime time.Time
	first := true

	for ssrc, stream := range jb.streams {
		stream.mu.RLock()
		lastRead := stream.lastReadTime
		stream.mu.RUnlock()

		if first || lastRead.Before(oldestTime) {
			oldestTime = lastRead
			oldestSSRC = ssrc
			first = false
		}
	}

	if !first {
		delete(jb.streams, oldestSSRC)
	}
}

func (sb *StreamBuffer) addPacket(packet *rtputils.RTPPacket) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	packet.ReceivedAt = time.Now()

	if _, exists := sb.packetsMap[packet.Sequence]; exists {
		return
	}

	if sb.packets.Len() >= sb.maxBufferSize {
		sb.cleanOldPackets()
	}

	sb.insertPacketSorted(packet)
}

func (sb *StreamBuffer) getPacket(bufferTime time.Duration) (*rtputils.RTPPacket, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.lastReadTime = time.Now()

	if sb.packets.Len() == 0 {
		return nil, nil
	}

	elem := sb.packets.Front()
	if elem == nil {
		return nil, nil
	}

	packet := elem.Value.(*rtputils.RTPPacket)

	if time.Since(packet.ReceivedAt) < bufferTime {
		return nil, nil
	}

	sb.packets.Remove(elem)
	delete(sb.packetsMap, packet.Sequence)

	sb.expectedSeq = packet.Sequence + 1

	return packet, nil
}

func (sb *StreamBuffer) insertPacketSorted(packet *rtputils.RTPPacket) {

	var element *list.Element
	for e := sb.packets.Front(); e != nil; e = e.Next() {
		p := e.Value.(*rtputils.RTPPacket)

		diff := int32(packet.Sequence) - int32(p.Sequence)

		if diff > 32767 {
			diff -= 65536
		} else if diff < -32767 {
			diff += 65536
		}

		if diff < 0 {
			element = sb.packets.InsertBefore(packet, e)
			sb.packetsMap[packet.Sequence] = element
			return
		}
	}

	element = sb.packets.PushBack(packet)
	sb.packetsMap[packet.Sequence] = element
}

func (sb *StreamBuffer) cleanOldPackets() {
	for sb.packets.Len() > sb.maxBufferSize/2 {
		elem := sb.packets.Front()
		if elem == nil {
			break
		}

		packet := elem.Value.(*rtputils.RTPPacket)
		sb.packets.Remove(elem)
		delete(sb.packetsMap, packet.Sequence)
	}
}

func (sb *StreamBuffer) calculatePacketDelay(packet *rtputils.RTPPacket) time.Duration {
	if sb.lastTimestamp == 0 {
		sb.lastTimestamp = packet.Timestamp
		return 0
	}

	timestampDiff := packet.Timestamp - sb.lastTimestamp
	sb.lastTimestamp = packet.Timestamp

	delayMs := float64(timestampDiff) / float64(sb.sampleRate) * 1000

	return time.Duration(delayMs) * time.Millisecond
}
