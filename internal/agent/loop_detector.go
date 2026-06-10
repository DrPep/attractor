package agent

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// LoopDetection reports whether the agent appears stuck.
type LoopDetection struct {
	IsLooping     bool
	Description   string
	PatternLength int
}

// LoopDetector spots repeated tool-call patterns within a sliding window.
type LoopDetector struct {
	window     []string
	windowSize int
	threshold  int
}

// NewLoopDetector returns a detector with the given window size and threshold.
func NewLoopDetector(windowSize, threshold int) *LoopDetector {
	if windowSize <= 0 {
		windowSize = 20
	}
	if threshold <= 0 {
		threshold = 3
	}
	return &LoopDetector{windowSize: windowSize, threshold: threshold}
}

// Record logs a tool call and checks for loops.
func (d *LoopDetector) Record(toolName string, args map[string]any) LoopDetection {
	sig := signature(toolName, args)
	d.window = append(d.window, sig)
	if len(d.window) > d.windowSize {
		d.window = d.window[len(d.window)-d.windowSize:]
	}

	w := d.window
	n := len(w)

	// Same call repeated threshold times consecutively.
	if n >= d.threshold {
		last := w[n-1]
		consecutive := 0
		for i := n - 1; i >= 0; i-- {
			if w[i] == last {
				consecutive++
			} else {
				break
			}
		}
		if consecutive >= d.threshold {
			return LoopDetection{IsLooping: true, PatternLength: 1,
				Description: fmt.Sprintf("Same tool call repeated %d times: %s", consecutive, toolName)}
		}
	}

	// Repeating sequences of length 2 and 3.
	for _, patternLen := range []int{2, 3} {
		if n >= patternLen*d.threshold {
			pattern := w[n-patternLen:]
			repeats := 0
			for i := 0; i < d.threshold; i++ {
				start := n - patternLen*(i+1)
				if start < 0 {
					break
				}
				if equalSlice(w[start:start+patternLen], pattern) {
					repeats++
				}
			}
			if repeats >= d.threshold {
				return LoopDetection{IsLooping: true, PatternLength: patternLen,
					Description: fmt.Sprintf("Repeating pattern of %d calls detected", patternLen)}
			}
		}
	}
	return LoopDetection{}
}

// Reset clears the detection window.
func (d *LoopDetector) Reset() { d.window = nil }

func signature(toolName string, args map[string]any) string {
	b, _ := json.Marshal(args)
	sum := md5.Sum(b)
	return toolName + ":" + hex.EncodeToString(sum[:])[:8]
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
