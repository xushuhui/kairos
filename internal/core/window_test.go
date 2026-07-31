package core

import "testing"

// 边界用例表来自 spec.md Testing Decisions / 06-highlight-window-algorithm.md。
func TestComputeWindow(t *testing.T) {
	cases := []struct {
		name                               string
		peakEndMs, targetLenMs, videoLenMs uint64
		wantStartMs, wantEndMs             uint64
	}{
		{"正常", 75_000, 60_000, 120_000, 15_000, 75_000},
		{"高潮在开头附近", 30_000, 60_000, 120_000, 0, 60_000},
		{"高潮在开头附近（短视频）", 30_000, 60_000, 50_000, 0, 50_000},
		{"视频短于目标时长", 20_000, 60_000, 45_000, 0, 45_000},
		{"精确边界：start == 0", 60_000, 60_000, 120_000, 0, 60_000},
		{"精确边界：start == 0（视频等于目标时长）", 60_000, 60_000, 60_000, 0, 60_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStart, gotEnd := ComputeWindow(c.peakEndMs, c.targetLenMs, c.videoLenMs)
			if gotStart != c.wantStartMs || gotEnd != c.wantEndMs {
				t.Errorf("ComputeWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
					c.peakEndMs, c.targetLenMs, c.videoLenMs,
					gotStart, gotEnd, c.wantStartMs, c.wantEndMs)
			}
		})
	}
}
