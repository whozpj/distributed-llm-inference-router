package model

import "time"

type Config struct {
	Base            time.Duration
	PrefillPerToken time.Duration
	DecodePerToken  time.Duration
}

type Costs struct {
	Prefill time.Duration
	Decode  time.Duration
	Total   time.Duration
}

type MockModel struct{ cfg Config }

func New(cfg Config) *MockModel { return &MockModel{cfg: cfg} }

func (m *MockModel) Costs(uncachedTokens, maxNewTokens int) Costs {
	prefill := time.Duration(uncachedTokens) * m.cfg.PrefillPerToken
	decode := time.Duration(maxNewTokens) * m.cfg.DecodePerToken
	return Costs{
		Prefill: prefill,
		Decode:  decode,
		Total:   m.cfg.Base + prefill + decode,
	}
}
