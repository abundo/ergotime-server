package main

import (
	"reflect"
	"testing"
)

func TestAcceptSingleDashConfig(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty", in: nil, want: nil},
		{
			name: "long flag unchanged",
			in:   []string{"ergotime-server", "--config", "a.yaml"},
			want: []string{"ergotime-server", "--config", "a.yaml"},
		},
		{
			name: "single dash flag",
			in:   []string{"ergotime-server", "-config", "/etc/ergotime/config.yaml"},
			want: []string{"ergotime-server", "--config", "/etc/ergotime/config.yaml"},
		},
		{
			name: "single dash equals",
			in:   []string{"ergotime-server", "-config=/etc/ergotime/config.yaml"},
			want: []string{"ergotime-server", "--config=/etc/ergotime/config.yaml"},
		},
		{
			name: "short flag unchanged",
			in:   []string{"ergotime-server", "-c", "a.yaml"},
			want: []string{"ergotime-server", "-c", "a.yaml"},
		},
		{
			name: "program name is not rewritten",
			in:   []string{"-config", "-config", "a.yaml"},
			want: []string{"-config", "--config", "a.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := acceptSingleDashConfig(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
