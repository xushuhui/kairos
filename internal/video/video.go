// Package video 封装 FFmpeg 子进程调用：音轨提取、CUDA 硬件加速剪辑、GPU 检测。
//
// 所有 ffmpeg 调用都通过 os/exec 经 PATH 查找 ffmpeg 可执行文件，不硬编码绝对
// 路径——09 号工单会把 ffmpeg.exe 与主程序一起打包到同一目录，届时该目录会被
// 加进 PATH，这里的调用方式不需要改动。
package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// 哨兵错误：区分 ffmpeg 失败的具体原因，供上层用 errors.Is() 判定，
// 不把 ffmpeg 的原始 stderr 直接抛给 UI。
var (
	// ErrFfmpegNotFound 表示 PATH 里找不到 ffmpeg 可执行文件。
	ErrFfmpegNotFound = errors.New("ffmpeg executable not found in PATH")
	// ErrInputUnreadable 表示输入文件不存在、无权限读取或不是合法的媒体文件。
	ErrInputUnreadable = errors.New("input file missing or unreadable")
	// ErrEncoderUnsupported 表示所选编码器在本机不可用（未编译进 ffmpeg、
	// 缺少对应硬件/驱动等）——CutClip 据此判断是否值得退化到 libx264 重试。
	ErrEncoderUnsupported = errors.New("encoder unsupported")
	// ErrEncodeFailed 表示 ffmpeg 启动成功但编码/写出阶段因其他原因失败
	// （磁盘写入失败等，不属于上面两类)。
	ErrEncodeFailed = errors.New("encode failed")
)

// nvidiaSmiTimeout 限制 nvidia-smi 探测调用的最长等待时间，避免驱动异常导致
// CudaAvailable() 挂起。
const nvidiaSmiTimeout = 5 * time.Second

// CudaAvailable 检测本机是否存在可用的 NVIDIA GPU + 驱动。
//
// 不能只看 ffmpeg 编译期是否包含 cuda hwaccel——那只代表 ffmpeg 二进制的编译
// 选项，跟本机是否真的插着 NVIDIA 显卡无关（例如这台开发机上的 Homebrew
// ffmpeg 就编译进了 cuda hwaccel，但机器上根本没有 NVIDIA 硬件）。真正可信的
// 信号是：PATH 里能找到 nvidia-smi，且真的能成功执行它。
func CudaAvailable() bool {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), nvidiaSmiTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path)
	return cmd.Run() == nil
}

// SelectEncoder 根据 CUDA 可用性选择视频编码器：CUDA 可用时用硬件编码器
// h264_nvenc，否则退化到软件编码器 libx264。
func SelectEncoder() string {
	if CudaAvailable() {
		return "h264_nvenc"
	}
	return "libx264"
}

// ExtractAudio 用 ffmpeg 从 videoPath 提取 16kHz 单声道 WAV，写到 outWav。
func ExtractAudio(videoPath, outWav string) error {
	args := []string{
		"-y",
		"-i", videoPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		outWav,
	}
	if err := runFfmpeg(args); err != nil {
		return fmt.Errorf("提取音轨失败: %w", err)
	}
	return nil
}

// CutClip 用视频编码器从 videoPath 截取 [startMs, endMs) 区间，写到 outPath。
//
// 编码器由 SelectEncoder() 选择；若选中的是硬件编码器 h264_nvenc 但编码过程
// 中失败（驱动异常、硬件不支持等），自动退化到 libx264 软编码重试一次，不
// 崩溃——这覆盖了"CUDA 检测通过但实际编码仍失败"的中途降级场景，不只是
// SelectEncoder() 那次静态选型判断。
func CutClip(videoPath string, startMs, endMs uint64, outPath string) error {
	if endMs <= startMs {
		return fmt.Errorf("剪辑失败: 无效的时间区间 [%d, %d)", startMs, endMs)
	}

	encoder := SelectEncoder()
	if err := runFfmpeg(cutClipArgs(videoPath, startMs, endMs, outPath, encoder)); err != nil {
		if encoder == "libx264" {
			return fmt.Errorf("剪辑失败: %w", err)
		}
		// 硬件编码器失败，退化到软件编码器重试一次。
		if fallbackErr := runFfmpeg(cutClipArgs(videoPath, startMs, endMs, outPath, "libx264")); fallbackErr != nil {
			return fmt.Errorf("剪辑失败（硬件编码器 %s 失败: %v；软件编码器 libx264 重试后仍失败）: %w", encoder, err, fallbackErr)
		}
	}
	return nil
}

// cutClipArgs 构造 CutClip 的 ffmpeg 参数：仅在使用 h264_nvenc 时前置
// -hwaccel cuda；起止时间转换为带毫秒精度的小数秒传给 -ss/-t。
func cutClipArgs(videoPath string, startMs, endMs uint64, outPath, encoder string) []string {
	startSec := float64(startMs) / 1000
	durSec := float64(endMs-startMs) / 1000

	var args []string
	if encoder == "h264_nvenc" {
		args = append(args, "-hwaccel", "cuda")
	}
	args = append(args,
		"-i", videoPath,
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-t", fmt.Sprintf("%.3f", durSec),
		"-c:v", encoder,
		"-c:a", "aac",
		"-y", outPath,
	)
	return args
}

// runFfmpeg 执行 ffmpeg 子进程，成功时返回 nil；失败时捕获 stderr 并归类为
// 具体的哨兵错误，附带 ffmpeg 报错的关键一行作为诊断信息。
func runFfmpeg(args []string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("%w: %v", ErrFfmpegNotFound, err)
	}

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return classifyFfmpegError(stderr.String())
	}
	return nil
}

// classifyFfmpegError 把 ffmpeg 的原始 stderr 归类为具体的哨兵错误，只附带
// 最能说明问题的一行文本，不把整段原始输出转发给调用方。
func classifyFfmpegError(stderr string) error {
	switch {
	case strings.Contains(stderr, "No such file or directory"),
		strings.Contains(stderr, "Invalid data found when processing input"),
		strings.Contains(stderr, "Permission denied"):
		return fmt.Errorf("%w: %s", ErrInputUnreadable, lastNonEmptyLine(stderr))
	case strings.Contains(stderr, "Unknown encoder"),
		strings.Contains(stderr, "Encoder not found"),
		strings.Contains(stderr, "No NVENC capable devices found"),
		strings.Contains(stderr, "Cannot load libnvidia-encode"),
		strings.Contains(stderr, "OpenEncodeSessionEx failed"):
		return fmt.Errorf("%w: %s", ErrEncoderUnsupported, lastNonEmptyLine(stderr))
	default:
		return fmt.Errorf("%w: %s", ErrEncodeFailed, lastNonEmptyLine(stderr))
	}
}

// lastNonEmptyLine 返回 stderr 里最后一行非空文本——ffmpeg 把最终结论性的
// 报错放在最后，比冗长的编译配置/版本信息更有诊断价值。
func lastNonEmptyLine(stderr string) string {
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}
