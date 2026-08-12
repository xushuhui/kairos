package asr

import (
	"reflect"
	"testing"

	"kairos/internal/core"
)

func TestMergeWordsToSentences(t *testing.T) {
	cases := []struct {
		name       string
		words      []WordToken
		endIndices []int
		want       []core.Sentence
	}{
		{
			name:       "空输入",
			words:      nil,
			endIndices: nil,
			want:       nil,
		},
		{
			name:       "单词成句",
			words:      []WordToken{{Text: "你好", StartMs: 0, EndMs: 500}},
			endIndices: []int{0},
			want: []core.Sentence{
				{ID: 0, StartMs: 0, EndMs: 500, Text: "你好"},
			},
		},
		{
			name: "多句合并",
			words: []WordToken{
				{Text: "你", StartMs: 0, EndMs: 100},
				{Text: "好", StartMs: 100, EndMs: 200},
				{Text: "吗", StartMs: 200, EndMs: 400},
				{Text: "我", StartMs: 500, EndMs: 600},
				{Text: "很", StartMs: 600, EndMs: 700},
				{Text: "好", StartMs: 700, EndMs: 900},
			},
			endIndices: []int{2},
			want: []core.Sentence{
				{ID: 0, StartMs: 0, EndMs: 400, Text: "你好吗"},
				{ID: 1, StartMs: 500, EndMs: 900, Text: "我很好"},
			},
		},
		{
			name: "边界正好是最后一个 token（不产生空尾句）",
			words: []WordToken{
				{Text: "甲", StartMs: 0, EndMs: 100},
				{Text: "乙", StartMs: 100, EndMs: 200},
			},
			endIndices: []int{1},
			want: []core.Sentence{
				{ID: 0, StartMs: 0, EndMs: 200, Text: "甲乙"},
			},
		},
		{
			name: "最后一句缺失句末标点时仍兜底成句",
			words: []WordToken{
				{Text: "甲", StartMs: 0, EndMs: 100},
				{Text: "乙", StartMs: 100, EndMs: 200},
				{Text: "丙", StartMs: 200, EndMs: 300},
			},
			endIndices: []int{0},
			want: []core.Sentence{
				{ID: 0, StartMs: 0, EndMs: 100, Text: "甲"},
				{ID: 1, StartMs: 100, EndMs: 300, Text: "乙丙"},
			},
		},
		{
			name: "越界/重复边界下标被忽略",
			words: []WordToken{
				{Text: "甲", StartMs: 0, EndMs: 100},
				{Text: "乙", StartMs: 100, EndMs: 200},
			},
			endIndices: []int{0, 0, -1, 99},
			want: []core.Sentence{
				{ID: 0, StartMs: 0, EndMs: 100, Text: "甲"},
				{ID: 1, StartMs: 100, EndMs: 200, Text: "乙"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mergeWordsToSentences(c.words, c.endIndices)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("mergeWordsToSentences(%+v, %v) = %+v, want %+v",
					c.words, c.endIndices, got, c.want)
			}
		})
	}
}

func TestSentenceEndIndicesFromPunctuation(t *testing.T) {
	cases := []struct {
		name       string
		words      []WordToken
		punctuated string
		want       []int
	}{
		{
			name:       "单句，句末标点定位到最后一个 token",
			words:      []WordToken{{Text: "你"}, {Text: "好"}},
			punctuated: "你好。",
			want:       []int{1},
		},
		{
			name: "多句，每个句末标点各定位一个边界",
			words: []WordToken{
				{Text: "你"}, {Text: "好"}, {Text: "吗"},
				{Text: "我"}, {Text: "很"}, {Text: "好"},
			},
			punctuated: "你好吗？我很好。",
			want:       []int{2, 5},
		},
		{
			name:       "没有标点时不产生边界",
			words:      []WordToken{{Text: "甲"}, {Text: "乙"}},
			punctuated: "甲乙",
			want:       nil,
		},
		{
			name:       "多字符 token 按 rune 长度正确推进边界",
			words:      []WordToken{{Text: "你好"}, {Text: "吗"}},
			punctuated: "你好吗？",
			want:       []int{1},
		},
		{
			name:       "空 words 不 panic，直接返回空边界",
			words:      nil,
			punctuated: "。",
			want:       nil,
		},
		{
			name:       "空 punctuated 字符串返回空边界",
			words:      []WordToken{{Text: "甲"}},
			punctuated: "",
			want:       nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sentenceEndIndicesFromPunctuation(c.words, c.punctuated)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("sentenceEndIndicesFromPunctuation(%+v, %q) = %v, want %v",
					c.words, c.punctuated, got, c.want)
			}
		})
	}
}
