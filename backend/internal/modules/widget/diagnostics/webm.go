package diagnostics

import (
	"encoding/binary"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"math"
)

// WebMDuration validates WebM container timestamps, including duration-less MediaRecorder
// streams. Only WebM video is accepted. No client-supplied duration is trusted.
// Bounded traversal rejects malformed, deeply nested or timestamp-less containers.
func WebMDuration(data []byte) (int, error) {
	if len(data) < 4 || binary.BigEndian.Uint32(data[:4]) != 0x1a45dfa3 {
		return 0, ErrInvalid
	}
	p := webmParser{scale: 1000000}
	if err := p.walk(data, 0); err != nil {
		return 0, err
	}
	if !p.webm || !p.video || !p.block || p.scale == 0 || p.scale > 1000000000 {
		return 0, ErrInvalid
	}
	ms := float64(p.maxTime)*float64(p.scale)/1000000 + float64(p.frameNS)/1000000
	if p.duration > ms {
		ms = p.duration
	}
	if math.IsNaN(ms) || math.IsInf(ms, 0) || ms < 0 || ms > 60000 {
		return 0, ErrInvalid
	}
	return int(math.Ceil(ms)), nil
}

type webmParser struct {
	scale              uint64
	cluster            uint64
	maxTime            int64
	duration           float64
	lastTime           int64
	frameNS            uint64
	webm, video, block bool
}

func vint(data []byte, strip bool) (uint64, int, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	mask := byte(0x80)
	n := 1
	for n <= 8 && data[0]&mask == 0 {
		mask >>= 1
		n++
	}
	if n > 8 || n > len(data) {
		return 0, 0, false
	}
	v := uint64(data[0])
	if strip {
		v = uint64(data[0] &^ mask)
	}
	for _, b := range data[1:n] {
		v = v<<8 | uint64(b)
	}
	return v, n, true
}
func uintValue(data []byte) (uint64, error) {
	if len(data) > 8 {
		return 0, ErrInvalid
	}
	var n uint64
	for _, b := range data {
		n = n<<8 | uint64(b)
	}
	return n, nil
}
func (p *webmParser) walk(data []byte, depth int) error {
	if depth > 8 {
		return ErrInvalid
	}
	for len(data) > 0 {
		id, a, ok := vint(data, false)
		if !ok || a > 4 {
			return ErrInvalid
		}
		size, b, ok := vint(data[a:], true)
		if !ok {
			return ErrInvalid
		}
		head := a + b
		unknown := size == (uint64(1)<<(7*b))-1
		if unknown {
			if id != 0x18538067 && id != 0x1f43b675 {
				return ErrInvalid
			}
			// MediaRecorder streams use unknown-sized segments/clusters. Continue
			// in the same bounded loop: following sibling clusters are not nested.
			if id == 0x1f43b675 {
				p.cluster = 0
			}
			data = data[head:]
			continue
		}
		if size > safeconv.ToUint64(len(data)-head) {
			return ErrInvalid
		}
		payload := data[head : head+safeconv.ToInt(size)]
		data = data[head+safeconv.ToInt(size):]
		switch id {
		case 0x1a45dfa3, 0x18538067, 0x1549a966, 0x1654ae6b, 0xae, 0x1f43b675, 0xa0:
			if id == 0x1f43b675 {
				p.cluster = 0
			}
			if err := p.walk(payload, depth+1); err != nil {
				return err
			}
		case 0x4282:
			p.webm = string(payload) == "webm"
		case 0x2ad7b1:
			v, err := uintValue(payload)
			if err != nil || v == 0 {
				return ErrInvalid
			}
			p.scale = v
		case 0x4489:
			var v float64
			switch len(payload) {
			case 4:
				v = float64(math.Float32frombits(binary.BigEndian.Uint32(payload)))
			case 8:
				v = math.Float64frombits(binary.BigEndian.Uint64(payload))
			default:
				return ErrInvalid
			}
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return ErrInvalid
			}
			p.duration = v * float64(p.scale) / 1000000
		case 0x83:
			v, err := uintValue(payload)
			if err != nil {
				return err
			}
			if v == 1 {
				p.video = true
			}
		case 0x23e383: // DefaultDuration is nanoseconds, independent of TimecodeScale.
			v, err := uintValue(payload)
			if err != nil || v > 60000000000 {
				return ErrInvalid
			}
			if v > p.frameNS {
				p.frameNS = v
			}
		case 0x9b: // BlockDuration is in timestamp ticks.
			v, err := uintValue(payload)
			if err != nil || v > 60000000 {
				return ErrInvalid
			}
			end := p.lastTime + safeconv.ToInt64(v)
			if end > p.maxTime {
				p.maxTime = end
			}
		case 0xe7:
			v, err := uintValue(payload)
			if err != nil || v > 60000000 {
				return ErrInvalid
			}
			p.cluster = v
		case 0xa3, 0xa1:
			_, n, ok := vint(payload, true)
			if !ok || len(payload) < n+4 {
				return ErrInvalid
			}
			relative := int64(binary.BigEndian.Uint16(payload[n : n+2]))
			if relative > 32767 {
				relative -= 65536
			}
			if p.cluster > 60000000 {
				return ErrInvalid
			}
			ts := safeconv.ToInt64(p.cluster) + relative
			if ts < 0 {
				return ErrInvalid
			}
			p.lastTime = ts
			if ts > p.maxTime {
				p.maxTime = ts
			}
			p.block = true
		}
	}
	return nil
}
