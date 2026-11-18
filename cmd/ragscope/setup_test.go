package main

import (
	"strings"
	"testing"

	"github.com/piovani/RAGscope/internal/config"
)

func TestPaidGeneratorsAreRefusedWithoutTheFlag(t *testing.T) {
	// A key in the environment must not be enough to spend money.
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	paid := []config.Generator{
		{Provider: "openai", Model: "m", PriceInputPerMTok: 1, PriceOutputPerMTok: 1},
		{Provider: "openai", Model: "m", BaseURL: "https://openrouter.ai/api/v1", PriceInputPerMTok: 1, PriceOutputPerMTok: 1},
		{Provider: "anthropic", Model: "m", PriceInputPerMTok: 1, PriceOutputPerMTok: 1},
	}

	allowPaid = false
	for _, g := range paid {
		_, err := newGenerator(g)
		if err == nil || !strings.Contains(err.Error(), "--allow-paid") {
			t.Errorf("%s %s: err = %v, want a refusal that mentions --allow-paid", g.Provider, g.BaseURL, err)
		}
	}

	allowPaid = true
	t.Cleanup(func() { allowPaid = false })
	for _, g := range paid {
		if _, err := newGenerator(g); err != nil {
			t.Errorf("%s with the flag: %v", g.Provider, err)
		}
	}
}

func TestFreeGeneratorsNeedNoFlag(t *testing.T) {
	allowPaid = false

	free := []config.Generator{
		{Provider: "ollama", Model: "llama3.2:3b"},
		{Provider: "openai", Model: "local", BaseURL: "http://localhost:1234/v1"},
	}
	for _, g := range free {
		if _, err := newGenerator(g); err != nil {
			t.Errorf("%s %s should run without the flag: %v", g.Provider, g.BaseURL, err)
		}
	}
}
