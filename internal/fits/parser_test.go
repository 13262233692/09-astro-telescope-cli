package fits

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fits")

	err := GenerateSampleFITSHeader(path)
	if err != nil {
		t.Fatalf("failed to generate sample: %v", err)
	}

	hdr, err := ParseHeaderFile(path)
	if err != nil {
		t.Fatalf("failed to parse header: %v", err)
	}

	if len(hdr.Cards) == 0 {
		t.Fatal("expected header cards, got none")
	}

	simple, ok := hdr.Get("SIMPLE")
	if !ok {
		t.Error("SIMPLE keyword not found")
	}
	if simple != "T" {
		t.Errorf("SIMPLE expected 'T', got '%s'", simple)
	}

	bitpix, err := hdr.GetInt("BITPIX")
	if err != nil {
		t.Errorf("BITPIX parse error: %v", err)
	}
	if bitpix != 16 {
		t.Errorf("BITPIX expected 16, got %d", bitpix)
	}

	naxis, err := hdr.GetInt("NAXIS")
	if err != nil {
		t.Errorf("NAXIS parse error: %v", err)
	}
	if naxis != 2 {
		t.Errorf("NAXIS expected 2, got %d", naxis)
	}

	telescope, ok := hdr.Get("TELESCOP")
	if !ok {
		t.Error("TELESCOP keyword not found")
	}
	if telescope != "VLA" {
		t.Errorf("TELESCOP expected 'VLA', got '%s'", telescope)
	}

	exptime, err := hdr.GetFloat("EXPTIME")
	if err != nil {
		t.Errorf("EXPTIME parse error: %v", err)
	}
	if exptime != 1800.0 {
		t.Errorf("EXPTIME expected 1800.0, got %f", exptime)
	}
}

func TestToCalibrationParams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fits")
	if err := GenerateSampleFITSHeader(path); err != nil {
		t.Fatalf("generate sample: %v", err)
	}

	hdr, err := ParseHeaderFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	params, err := hdr.ToCalibrationParams()
	if err != nil {
		t.Fatalf("to calibration params: %v", err)
	}

	if params.Telescope != "VLA" {
		t.Errorf("telescope: expected VLA, got %s", params.Telescope)
	}
	if params.Instrument != "EVLA" {
		t.Errorf("instrument: expected EVLA, got %s", params.Instrument)
	}
	if params.Exposure != 1800.0 {
		t.Errorf("exposure: expected 1800.0, got %f", params.Exposure)
	}
	if params.Gain != 2.5 {
		t.Errorf("gain: expected 2.5, got %f", params.Gain)
	}
	if len(params.Custom) == 0 {
		t.Error("expected custom fields")
	}
}

func TestToMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fits")
	if err := GenerateSampleFITSHeader(path); err != nil {
		t.Fatal(err)
	}
	hdr, err := ParseHeaderFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := hdr.ToMap()
	if len(m) == 0 {
		t.Error("expected map entries")
	}
	if m["OBJECT"] != "M31" {
		t.Errorf("OBJECT: expected M31, got %s", m["OBJECT"])
	}
}

func TestGenerateSampleFITSHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.fits")
	err := GenerateSampleFITSHeader(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("file not created")
	}
}
