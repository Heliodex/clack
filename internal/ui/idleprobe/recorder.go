package idleprobe

import (
	"fmt"
	"maps"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gioui.org/io/input"
	"gioui.org/op"
)

// Recorder watches a real window (App.Run's, the huddle window's) the way
// Probe watches a simulated one: the frames it draws and who asked for
// each, plus the process's CPU time and memory over the same period.
//
// The window's own loop runs: call Frame as every FrameEvent arrives
// (before its layout) and Invalidated from the window's invalidate path.
// Every op.InvalidateCmd a frame executes is recorded through Gio's
// input.ExecuteHook; a frame's causes are the previous frame's requests
// that were due when it came, and the Invalidates since.
type Recorder struct {
	mu    sync.Mutex
	on    bool
	start time.Time
	cpu0  time.Duration
	mem0  memSample
	cur   []cmd          // the InvalidateCmds since the last frame began
	invs  map[string]int // Invalidate callers since the last frame
	res   Result
	// over the whole period, frames or not (a hidden window draws
	// none but still wakes for every Invalidate)
	allInvs map[string]int
	allCmds map[string]int
}

// NewRecorder makes a recorder and points Gio's ExecuteHook at it (one
// probe or recorder per process at a time).
func NewRecorder() *Recorder {
	r := &Recorder{invs: map[string]int{}, allInvs: map[string]int{}, allCmds: map[string]int{}}
	input.ExecuteHook = func(c input.Command) {
		ic, ok := c.(op.InvalidateCmd)
		if !ok || !r.recording() { // no stack walks outside a period
			return
		}
		who := Caller(1)
		now := time.Now()
		r.mu.Lock()
		r.cur = append(r.cur, cmd{at: ic.At, who: who})
		if r.on {
			switch {
			case ic.At.IsZero():
				r.allCmds["InvalidateCmd{} "+who]++
			case !ic.At.After(now.Add(Vsync)):
				r.allCmds["InvalidateCmd (due within a vsync) "+who]++
			default:
				r.allCmds["InvalidateCmd (later) "+who]++
			}
		}
		r.mu.Unlock()
	}
	return r
}

// Close detaches the recorder from Gio.
func (r *Recorder) Close() { input.ExecuteHook = nil }

func (r *Recorder) recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.on
}

// Start begins a period.
func (r *Recorder) Start(name string) {
	cpu := ProcessCPU()
	mem := sampleMem()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.on = true
	r.start = time.Now()
	r.cpu0 = cpu
	r.mem0 = mem
	clear(r.invs)
	clear(r.allInvs)
	clear(r.allCmds)
	r.res = Result{Name: name}
}

// Stop ends the period and reports it.
func (r *Recorder) Stop() Report {
	cpu := ProcessCPU()
	mem := sampleMem()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.on = false
	res := r.res
	res.Dur = time.Since(r.start)
	return Report{Result: res, CPU: cpu - r.cpu0, Mem0: r.mem0, Mem1: mem,
		Invalidates: maps.Clone(r.allInvs), Cmds: maps.Clone(r.allCmds)}
}

// Frame records a FrameEvent (the window's goroutine, as it arrives).
func (r *Recorder) Frame() {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.on {
		seen := map[string]bool{}
		var causes []string
		for _, c := range r.cur {
			if c.at.After(now.Add(Vsync / 2)) {
				continue
			}
			s := "InvalidateCmd " + c.who
			if c.at.IsZero() {
				s = "InvalidateCmd{} " + c.who
			}
			if !seen[s] {
				seen[s] = true
				causes = append(causes, s)
			}
		}
		for who, n := range r.invs {
			c := "Invalidate " + who
			if n > 1 {
				c += fmt.Sprintf(" (×%d)", n)
			}
			causes = append(causes, c)
		}
		if len(causes) == 0 {
			causes = append(causes, "no request seen (input, the OS, or a request made before the period)")
		}
		sort.Strings(causes)
		r.res.Frames = append(r.res.Frames, Frame{At: now.Sub(r.start), Causes: causes})
	}
	r.cur = r.cur[:0]
	clear(r.invs)
}

// Invalidated records a window Invalidate (any goroutine).
func (r *Recorder) Invalidated() {
	if !r.recording() {
		return
	}
	who := Caller(1)
	r.mu.Lock()
	r.invs[who]++
	if r.on {
		r.allInvs[who]++
	}
	r.mu.Unlock()
}

// Report is a recorded period: the frames and their causes, the CPU the
// whole process used and its memory before and after.
type Report struct {
	Result
	CPU        time.Duration
	Mem0, Mem1 memSample
	// every Invalidate and InvalidateCmd in the period by caller, drawn
	// or not
	Invalidates, Cmds map[string]int
}

// CPUShare is the process's CPU over the period as a share of one core.
func (r Report) CPUShare() float64 {
	if r.Dur <= 0 {
		return 0
	}
	return float64(r.CPU) / float64(r.Dur)
}

// Log writes the CPU and memory line, then Result.Log's frames.
func (r Report) Log(t interface {
	Helper()
	Log(...any)
}) {
	t.Helper()
	t.Log(r.String())
}

// String is everything Log writes.
func (r Report) String() string {
	var sb strings.Builder
	sb.WriteString(r.Summary() + "\n")
	writeCounts(&sb, "Invalidate", r.Invalidates)
	writeCounts(&sb, "InvalidateCmd", r.Cmds)
	r.Result.write(&sb)
	return sb.String()
}

func writeCounts(sb *strings.Builder, what string, m map[string]int) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || m[keys[i]] == m[keys[j]] && keys[i] < keys[j] })
	n := 0
	for _, k := range keys {
		n += m[k]
	}
	fmt.Fprintf(sb, "  %s calls in the period: %d\n", what, n)
	for i, k := range keys {
		if i == 15 {
			fmt.Fprintf(sb, "    … %d more callers\n", len(keys)-i)
			break
		}
		fmt.Fprintf(sb, "    %6d  %s\n", m[k], k)
	}
}

// Summary is the period in one line.
func (r Report) Summary() string {
	mb := func(b uint64) float64 { return float64(b) / (1 << 20) }
	return fmt.Sprintf("idle-cpu %s: %d frames in %v, CPU %v (%.1f%% of a core), Go heap %.1f → %.1f MB, goroutines %d → %d",
		r.Name, len(r.Frames), r.Dur.Round(time.Millisecond), r.CPU.Round(time.Millisecond), 100*r.CPUShare(),
		mb(r.Mem0.heap), mb(r.Mem1.heap), r.Mem0.goroutines, r.Mem1.goroutines)
}

type memSample struct {
	heap       uint64
	goroutines int
}

func sampleMem() memSample {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memSample{heap: m.HeapAlloc, goroutines: runtime.NumGoroutine()}
}
