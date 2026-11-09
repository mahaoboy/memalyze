package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mahaoboy/memalyze/internal/store"
	"github.com/mahaoboy/memalyze/internal/store/sqlite"
)

func runAuditVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "write machine-readable JSON")
	checkpointPath := fs.String("checkpoint", "", "trusted checkpoint to compare (exact event count and head)")
	writeCheckpointPath := fs.String("write-checkpoint", "", "write a new checkpoint (must not exist)")
	dbPath := fs.String("db", "", "database path (default MEMALYZE_DB)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "memalyze: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	expected, err := readAuditCheckpoint(*checkpointPath)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: read checkpoint: %v\n", err)
		return 1
	}
	ctx := context.Background()
	st, err := sqlite.Open(ctx, dbPathOr(*dbPath))
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: open database: %v\n", err)
		return 1
	}
	defer st.Close()

	v, err := st.VerifyAudit(ctx, expected)
	if err != nil {
		fmt.Fprintf(stderr, "memalyze: verify audit: %v\n", err)
		return 1
	}
	if *writeCheckpointPath != "" && v.OK {
		cp, _ := v.Checkpoint()
		if err := writeAuditCheckpointNew(*writeCheckpointPath, cp); err != nil {
			fmt.Fprintf(stderr, "memalyze: write checkpoint: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "memalyze: checkpoint written to %s (mode 0600); store it separately from the database\n", *writeCheckpointPath)
	}

	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			fmt.Fprintf(stderr, "memalyze: write verification: %v\n", err)
			return 1
		}
	} else {
		var writeErr error
		if v.OK {
			_, writeErr = fmt.Fprintf(stdout, "audit ok: %d events, last_seq=%d, head=%s, checkpoint=%s\n",
				v.EventCount, v.LastSeq, v.Head, v.CheckpointStatus)
			if writeErr == nil && v.CheckpointStatus == "not_provided" {
				_, writeErr = fmt.Fprintln(stdout, "internal consistency only; compare a trusted checkpoint to detect tail truncation")
			}
		} else {
			_, writeErr = fmt.Fprintf(stdout, "audit failed: reason=%s first_invalid_seq=%d events=%d head=%s checkpoint=%s\n",
				v.Reason, v.FirstInvalidSeq, v.EventCount, v.Head, v.CheckpointStatus)
		}
		if writeErr != nil {
			fmt.Fprintf(stderr, "memalyze: write verification: %v\n", writeErr)
			return 1
		}
	}
	if !v.OK {
		if *writeCheckpointPath != "" {
			fmt.Fprintln(stderr, "memalyze: refusing to write a checkpoint for a failed verification")
		}
		return 1
	}
	return 0
}

func readAuditCheckpoint(path string) (*store.AuditCheckpoint, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("checkpoint %s exceeds 1 MiB", path)
	}
	var cp store.AuditCheckpoint
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&cp); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if cp.SchemaVersion != store.AuditChainVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", cp.SchemaVersion)
	}
	if cp.Algorithm != store.AuditChainAlgorithm {
		return nil, fmt.Errorf("unsupported algorithm %q", cp.Algorithm)
	}
	if cp.EventCount < 0 || cp.LastSeq != cp.EventCount {
		return nil, fmt.Errorf("invalid event_count/last_seq %d/%d", cp.EventCount, cp.LastSeq)
	}
	if len(cp.Head) != 64 {
		return nil, fmt.Errorf("head must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(cp.Head); err != nil {
		return nil, fmt.Errorf("invalid head: %w", err)
	}
	if cp.EventCount == 0 && cp.Head != store.AuditGenesisHash {
		return nil, fmt.Errorf("empty checkpoint must use the genesis head")
	}
	return &cp, nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeAuditCheckpointNew(path string, cp store.AuditCheckpoint) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cp); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
