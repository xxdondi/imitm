package modules

import (
	"reflect"
	"testing"

	"github.com/elazarl/goproxy"
)

type testModule struct{ name string }

func (m testModule) Name() string                    { return m.name }
func (testModule) Register(*goproxy.ProxyHttpServer) {}

func TestSelect(t *testing.T) {
	available := []Module{testModule{"first"}, testModule{"second"}}
	for _, tc := range []struct {
		selection string
		want      []string
		wantErr   bool
	}{
		{"", nil, false},
		{"  ", nil, false},
		{" second, first ", []string{"second", "first"}, false},
		{"first,missing", nil, true},
		{"first,first", nil, true},
		{"first,", nil, true},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			selected, err := Select(tc.selection, available...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			var names []string
			for _, m := range selected {
				names = append(names, m.Name())
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("selection = %v, want %v", names, tc.want)
			}
		})
	}
	if _, err := Select("first", testModule{"first"}, testModule{"first"}); err == nil {
		t.Fatal("accepted duplicate available modules")
	}
}
