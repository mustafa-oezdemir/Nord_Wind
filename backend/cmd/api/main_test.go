package main

import "testing"

func TestDemoCheckoutEnabled(t *testing.T) {
	for _, test := range []struct {
		environment string
		flag        string
		want        bool
	}{
		{"development", "true", true},
		{"development", "", false},
		{"development", "TRUE", false},
		{"production", "true", false},
		{"staging", "true", false},
	} {
		if got := demoCheckoutEnabled(test.environment, test.flag); got != test.want {
			t.Errorf("environment=%q flag=%q: got %v, want %v", test.environment, test.flag, got, test.want)
		}
	}
}
