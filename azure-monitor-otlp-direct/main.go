package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	inputPath := flag.String("input", "", "path to an OTLP JSON ExportLogsServiceRequest")
	outputPath := flag.String("output", "", "path for the encoded OTLP protobuf request")
	flag.Parse()

	if *inputPath == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "both --input and --output are required")
		os.Exit(2)
	}

	jsonPayload, err := os.ReadFile(*inputPath)
	if err != nil {
		exitf("read OTLP JSON payload: %v", err)
	}

	request := new(collectorlogsv1.ExportLogsServiceRequest)
	if err := protojson.Unmarshal(jsonPayload, request); err != nil {
		exitf("decode OTLP JSON payload: %v", err)
	}

	protobufPayload, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		exitf("encode OTLP protobuf payload: %v", err)
	}

	if err := writeAtomically(*outputPath, protobufPayload); err != nil {
		exitf("write OTLP protobuf payload: %v", err)
	}

	digest := sha256.Sum256(protobufPayload)
	fmt.Printf("Encoded OTLP protobuf payload: %s\n", *outputPath)
	fmt.Printf("OTLP protobuf bytes: %d\n", len(protobufPayload))
	fmt.Printf("OTLP protobuf SHA-256: %s\n", hex.EncodeToString(digest[:]))
}

func writeAtomically(outputPath string, payload []byte) error {
	outputDir := filepath.Dir(outputPath)
	tempFile, err := os.CreateTemp(outputDir, ".payload-*.pb")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(payload); err != nil {
		tempFile.Close()
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, outputPath)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
