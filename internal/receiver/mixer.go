package receiver

import (
	"sync"
	"time"
)

type participant struct {
	lastFrame []int16
	lastSeen  time.Time
	sequence  uint32
}

type mixer struct {
	participants map[string]*participant
	mu           sync.RWMutex
	frameSize    int
}

func NewMixer(frameSize int) *mixer {
	return &mixer{
		participants: make(map[string]*participant),
		frameSize:    frameSize,
	}
}

func (m *mixer) AddFrame(addr string, sequence uint32, samples []int16) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.participants[addr]
	if !exists {
		p = &participant{
			lastFrame: make([]int16, m.frameSize),
			lastSeen:  time.Now(),
		}
		m.participants[addr] = p
	}

	if sequence >= p.sequence {
		if len(samples) > 0 {
			copyLen := len(samples)
			if copyLen > m.frameSize {
				copyLen = m.frameSize
			}
			copy(p.lastFrame[:copyLen], samples[:copyLen])
		}
		p.sequence = sequence
		p.lastSeen = time.Now()
	}
}

func (m *mixer) Mix() []int16 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeDeadline := time.Now().Add(-3 * time.Second)
	var activeFrames [][]int16

	for _, p := range m.participants {
		if p.lastSeen.After(activeDeadline) && len(p.lastFrame) > 0 {
			activeFrames = append(activeFrames, p.lastFrame)
		}
	}

	if len(activeFrames) == 0 {
		return make([]int16, m.frameSize)
	}

	if len(activeFrames) == 1 {
		result := make([]int16, m.frameSize)
		copy(result, activeFrames[0])
		return result
	}

	return m.mixFrames(activeFrames)
}

func (m *mixer) mixFrames(frames [][]int16) []int16 {
	result := make([]int16, m.frameSize)

	for i := 0; i < m.frameSize; i++ {
		var sum int32 = 0
		var count int32 = 0

		for _, frame := range frames {
			if i < len(frame) {
				sum += int32(frame[i])
				count++
			}
		}

		if count > 0 {
			avg := sum / count

			if avg > 32767 {
				avg = 32767
			} else if avg < -32768 {
				avg = -32768
			}

			result[i] = int16(avg)
		}
	}

	return result
}

func (m *mixer) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	inactiveDeadline := time.Now().Add(-10 * time.Second)
	for addr, p := range m.participants {
		if p.lastSeen.Before(inactiveDeadline) {
			delete(m.participants, addr)
		}
	}
}
