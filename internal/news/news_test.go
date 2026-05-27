package news

import "testing"

func TestScoreText_Positive(t *testing.T) {
	cases := []string{
		"NVIDIA shares surge on strong earnings beat",
		"Analyst upgrade boosts SOXL rally",
		"半導体株が急騰、好調な決算で上昇",
	}
	for _, c := range cases {
		if s := scoreText(c); s <= 0 {
			t.Errorf("expected positive score for %q, got %d", c, s)
		}
	}
}

func TestScoreText_Negative(t *testing.T) {
	cases := []string{
		"Chip stocks plunge after weak guidance",
		"Downgrade triggers SOXS rally on bearish outlook",
		"半導体株が急落、減益で下落",
	}
	for _, c := range cases {
		if s := scoreText(c); s >= 0 {
			t.Errorf("expected negative score for %q, got %d", c, s)
		}
	}
}

func TestScoreText_Neutral(t *testing.T) {
	if s := scoreText("Company announces new product line"); s != 0 {
		t.Errorf("expected 0, got %d", s)
	}
}

func TestSentimentLabel(t *testing.T) {
	if sentimentLabel(2) != "🟢 Positive" {
		t.Error("positive label wrong")
	}
	if sentimentLabel(-2) != "🔴 Negative" {
		t.Error("negative label wrong")
	}
	if sentimentLabel(0) != "⚪️ Neutral" {
		t.Error("neutral label wrong")
	}
}

func TestScoreText_CaseInsensitive(t *testing.T) {
	if scoreText("STOCKS SURGE") <= 0 {
		t.Error("upper-case not detected")
	}
}
