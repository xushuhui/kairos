package core

// ComputeWindow 把 LLM 判定出的高光窗口结束时间锚定为片段末尾，向回倒推
// targetLenMs 得到起始时间，形成短剧标准的"悬念式收尾"效果。
//
// 两条边界 clamp：
//   - videoLenMs <= targetLenMs：视频本身比目标时长还短，直接返回整段视频。
//   - start < 0（高潮点太靠近视频开头）：clamp 到 0，end 取 targetLenMs——
//     此分支下高潮点不再落在片段结尾，接受这个妥协，不做复杂逻辑调回。
//     （上面的 videoLenMs <= targetLenMs 已提前返回，走到这里时
//     videoLenMs > targetLenMs 恒成立，故 end 直接取 targetLenMs 即可，
//     不需要再和 videoLenMs 取 min）
//
// 算法推导与边界用例详见
// docs/scratch/short-drama-highlight-clip/06-highlight-window-algorithm.md。
func ComputeWindow(peakEndMs, targetLenMs, videoLenMs uint64) (startMs, endMs uint64) {
	if videoLenMs <= targetLenMs {
		return 0, videoLenMs
	}

	start := int64(peakEndMs) - int64(targetLenMs)
	if start < 0 {
		return 0, targetLenMs
	}

	return uint64(start), peakEndMs
}
