package pmtool

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func TestForgeArchiveChecksWholeGzipStreamAndExpansionLimit(t *testing.T) {
	metadata := `{"name":"team-sample","version":"1.2.3"}`
	valid := forgeFixture(t, metadata, 1)
	corrupt := bytes.Clone(valid)
	corrupt[len(corrupt)-8] ^= 0xff

	reader, err := gzip.NewReader(bytes.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 64<<10)
	for size := 0; size < maximumForgeArchive; size += len(block) {
		if _, err := writer.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	for _, fixture := range []struct {
		name   string
		body   []byte
		accept bool
	}{
		{"valid", valid, true},
		{"checksum", corrupt, false},
		{"truncated footer", valid[:len(valid)-8], false},
		{"trailing garbage", append(bytes.Clone(valid), []byte("invalid gzip member")...), false},
		{"oversized expansion after tar EOF", output.Bytes(), false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			err := verifyForgeArchive(fixture.body, "team-sample-1.2.3", forgeMetadata{Name: "team-sample", Version: "1.2.3"})
			if (err == nil) != fixture.accept {
				t.Fatalf("archive integrity acceptance mismatch: %v", err)
			}
		})
	}
}
