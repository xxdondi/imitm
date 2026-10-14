package youtube

import (
	"bytes"
	"google.golang.org/protobuf/encoding/protowire"
	"os"
	"path/filepath"
	"testing"
)

func adField(num protowire.Number, payload []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, num, protowire.BytesType), payload)
}

func playerAdsFixture() ([]byte, []byte) {
	content := append(adField(4, adField(15, []byte("signed content URL"))), adField(11, adField(1, []byte("CpP6JbG7mBE")))...)
	content = append(content, adField(99, []byte("opaque unknown content"))...)
	ads := append(adField(7, []byte("legacy ad")), adField(68, adField(424701016, []byte("/pagead/")))...)
	ui := adField(1, adField(3, adField(7, []byte("UI field 7 preserved"))))
	return append(adField(1, adField(2, append(bytes.Clone(content), ads...))), ui...), append(adField(1, adField(2, content)), ui...)
}

func fixturePlayer(data []byte) []byte {
	f, _ := wireFields(data)
	envelope := data[f[0].payloadStart:f[0].end]
	f, _ = wireFields(envelope)
	return envelope[f[0].payloadStart:f[0].end]
}

func TestRootPlayerAdsRemoval(t *testing.T) {
	original, want := playerAdsFixture()
	out, changes, err := removePlayerAdsAt(fixturePlayer(original), 2)
	if err != nil || len(changes) != 2 || !bytes.Equal(out, fixturePlayer(want)) {
		t.Fatalf("root removal: %+v %v", changes, err)
	}
	for _, tc := range []struct {
		file   string
		hasAds bool
	}{
		{"000971-response.bin", true},
		{"000961-response.bin", false},
		{"000168-response.bin", false},
	} {
		input, err := os.ReadFile(filepath.Join("../../../data/captures/20261014-145022-2249589943", tc.file))
		if os.IsNotExist(err) {
			t.Log("local capture unavailable", tc.file)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out, changes, err := removePlayerAdsAt(input, 2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(playerAdsPreservedView(input, 2), playerAdsPreservedView(out, 2)) {
			t.Fatal("root content changed")
		}
		if tc.hasAds {
			if len(changes) == 0 || bytes.Contains(out, []byte("lHSzl7V25nk")) || bytes.Contains(out, []byte("0OvC3acTDEM")) {
				t.Fatal("fallback ads remain")
			}
		} else if len(changes) != 0 || !bytes.Equal(out, input) {
			t.Fatal("ad-free fallback changed")
		}
	}
}

func TestPlayerAdsRemoval(t *testing.T) {
	input, want := playerAdsFixture()
	out, changes, err := removePlayerAds(input)
	if err != nil || len(changes) != 2 || !bytes.Equal(out, want) {
		t.Fatalf("bad removal: %+v %v", changes, err)
	}
	again, changes, err := removePlayerAds(out)
	if err != nil || len(changes) != 0 || !bytes.Equal(out, again) {
		t.Fatal("not idempotent")
	}
	for _, input := range [][]byte{append(bytes.Clone(input), 0xff), adField(1, adField(2, []byte{0xff})), adField(1, adField(2, adField(68, adField(424701016, []byte("/pagead/")))))} {
		out, changes, _ := removePlayerAds(input)
		if len(changes) != 0 || !bytes.Equal(input, out) {
			t.Fatal("invalid/unsupported input changed")
		}
	}
}

func playerAdsPreservedView(data []byte, depth int) []byte {
	fields, valid := wireFields(data)
	if !valid {
		panic("invalid rewritten message")
	}
	var out []byte
	for _, f := range fields {
		if depth == 2 && (f.num == 7 || f.num == 68) {
			continue
		}
		if (depth == 0 && f.num == 1) || (depth == 1 && f.num == 2) {
			out = append(out, playerAdsPreservedView(data[f.payloadStart:f.end], depth+1)...)
		} else {
			out = append(out, data[f.start:f.end]...)
		}
	}
	return out
}

func TestPlayerAdsCapturedPreservation(t *testing.T) {
	for _, path := range []string{
		"../../../data/captures/20261014-141153-3894257705/000634-response.bin",
		"../../../data/captures/20261014-142633-3068603029/000071-instantskip-original.bin",
		"../../../data/captures/20261014-141153-3894257705/000448-response.bin",
	} {
		input, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			t.Log("local capture unavailable", path)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out, changes, err := removePlayerAds(input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(playerAdsPreservedView(input, 0), playerAdsPreservedView(out, 0)) {
			t.Fatal("changed content metadata or UI")
		}
		if filepath.Base(path) == "000448-response.bin" {
			if len(changes) != 0 || !bytes.Equal(input, out) {
				t.Fatal("Premium changed")
			}
		} else {
			if len(changes) == 0 {
				t.Fatal("no ads removed")
			}
			t.Logf("%s: %d fields; %d -> %d bytes", path, len(changes), len(input), len(out))
		}
	}
}
