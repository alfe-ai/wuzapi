package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestProfileNameRequest(t *testing.T) {
	got, err := (profileNameRequest{Name: "  Alfred  "}).name()
	if err != nil || got != "Alfred" {
		t.Fatalf("expected trimmed name, got %q, %v", got, err)
	}
	if _, err := (profileNameRequest{Name: strings.Repeat("א", maxProfileNameLength)}).name(); err != nil {
		t.Fatalf("rejected name at the limit: %v", err)
	}
	for _, name := range []string{"", "   ", strings.Repeat("a", maxProfileNameLength+1)} {
		if _, err := (profileNameRequest{Name: name}).name(); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
}

func TestProfilePhotoRequest(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	dataURL := func(mime string, b []byte) string {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	}

	got, err := (profilePhotoRequest{Image: dataURL("image/jpeg", jpeg)}).jpeg()
	if err != nil || string(got) != string(jpeg) {
		t.Fatalf("expected decoded JPEG, got %v, %v", got, err)
	}
	if got, err := (profilePhotoRequest{}).jpeg(); err != nil || got != nil {
		t.Fatalf("empty Image should mean remove (nil, nil), got %v, %v", got, err)
	}
	for _, image := range []string{
		dataURL("image/png", png),
		base64.StdEncoding.EncodeToString(jpeg),
		"data:image/jpeg;base64,!!!",
	} {
		if _, err := (profilePhotoRequest{Image: image}).jpeg(); err == nil {
			t.Fatalf("accepted invalid image %q", image)
		}
	}
}
