package main

import (
	"io"
	"os"
	"testing"
)

func TestRemoveAds(t *testing.T) {
	inFile, err := os.Open("test_data/youtube-videoads.pb.bin")
	if err != nil {
		t.Fatalf("Error reading test data - %v", err)
	}
	defer inFile.Close()
	inBytes, err := io.ReadAll(inFile)
	if err != nil {
		t.Fatalf("Error reading test data - %v", err)
	}
	RemoveAds(inBytes)
}
