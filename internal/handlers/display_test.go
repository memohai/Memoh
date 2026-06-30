package handlers

import (
	"context"
	"reflect"
	"testing"
)

func TestResolveDisplayHostIPsStripsPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want []string
	}{
		{name: "plain ipv4", host: "100.123.2.67", want: []string{"100.123.2.67"}},
		{name: "ipv4 port", host: "100.123.2.67:8082", want: []string{"100.123.2.67"}},
		{name: "bracketed ipv6 port", host: "[::1]:8082", want: []string{"::1"}},
		{name: "plain ipv6", host: "::1", want: []string{"::1"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveDisplayHostIPs(context.Background(), tt.host); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolveDisplayHostIPs(%q) = %#v, want %#v", tt.host, got, tt.want)
			}
		})
	}
}

func TestFirstHeaderValue(t *testing.T) {
	t.Parallel()

	got := firstHeaderValue("100.123.2.67, 10.0.0.2")
	if got != "100.123.2.67" {
		t.Fatalf("firstHeaderValue returned %q", got)
	}
}
