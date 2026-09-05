package mobile

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// An iOS Packet Tunnel is jetsam-killed around 15 MB for the whole extension,
// Swift included. The default Go GC waits until the heap doubles, which is far
// too late here: sessions died after 9-19 minutes of traffic with no reconnect
// callback ever running, i.e. the process was gone.
const defaultExtensionLimitMB = 12

// TuneForExtension caps the Go heap and makes the collector far more eager.
// limitMB <= 0 uses the default. Safe to call more than once.
func TuneForExtension(limitMB int) {
	if limitMB <= 0 {
		limitMB = defaultExtensionLimitMB
	}
	debug.SetMemoryLimit(int64(limitMB) << 20)
	// 40%: collect often enough for a Packet Tunnel, but a speedtest burst
	// must not stall the QUIC thread in a GC storm (20% did).
	debug.SetGCPercent(40)
}

// HeapKB is the live Go heap in KiB. For on-screen diagnostics.
func HeapKB() int {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int(m.HeapAlloc >> 10)
}

// ReleaseMemory returns free pages to the OS. Called on idle/reconnect.
func ReleaseMemory() {
	debug.FreeOSMemory()
}

// packetBufSize covers any MTU we allow (1500) plus headroom.
const packetBufSize = 2048

// bufPool recycles per-packet buffers. Every packet used to be a fresh
// append([]byte(nil), ...), so a few Mbit of traffic churned megabytes and
// pushed the extension into the jetsam limit.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, packetBufSize)
		return &b
	},
}

// getBuf returns a buffer holding a copy of src. Release it with putBuf.
func getBuf(src []byte) *[]byte {
	if len(src) > packetBufSize {
		b := append([]byte(nil), src...)
		return &b
	}
	p := bufPool.Get().(*[]byte)
	b := (*p)[:len(src)]
	copy(b, src)
	*p = b
	return p
}

func putBuf(p *[]byte) {
	if p == nil || cap(*p) != packetBufSize {
		return
	}
	*p = (*p)[:packetBufSize]
	bufPool.Put(p)
}
