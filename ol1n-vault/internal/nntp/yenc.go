package nntp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
)

// SegmentSize is the default payload size of a single UseNet article before
// yEnc expansion. 750 000 bytes is the de-facto standard.
const SegmentSize = 750_000

// yEncLineLen is the target number of raw (pre-encoded) bytes per line.
// The actual output line may be slightly longer because of escapes.
const yEncLineLen = 128

// YEncEncodeSegment encodes a single binary segment into a yEnc article body.
//
// Args:
//   - filename: name advertised in =ybegin header
//   - part: 1-based part number, 0 for single-part articles
//   - total: total number of parts (only relevant when part > 0)
//   - totalSize: size of the entire original file (ybegin size=)
//   - partBegin: 1-based inclusive byte offset of this part in the file
//   - data: raw bytes of this part (len(data) is ypart end-begin+1)
//   - fileCRC: optional full-file CRC32 to emit on the final part (0 to omit)
func YEncEncodeSegment(filename string, part, total int, totalSize int64, partBegin int64, data []byte, fileCRC uint32) []byte {
	var buf bytes.Buffer
	if part > 0 {
		fmt.Fprintf(&buf, "=ybegin part=%d total=%d line=%d size=%d name=%s\r\n",
			part, total, yEncLineLen, totalSize, filename)
		fmt.Fprintf(&buf, "=ypart begin=%d end=%d\r\n",
			partBegin, partBegin+int64(len(data))-1)
	} else {
		fmt.Fprintf(&buf, "=ybegin line=%d size=%d name=%s\r\n",
			yEncLineLen, totalSize, filename)
	}

	col := 0
	for i, b := range data {
		out := byte((int(b) + 42) % 256)
		escape := false
		switch out {
		case 0x00, 0x0A, 0x0D, 0x3D:
			escape = true
		}
		// Escape TAB/SPACE at beginning or end of a line.
		atLineStart := col == 0
		atLineEnd := col+1 >= yEncLineLen || i == len(data)-1
		if !escape && (out == 0x09 || out == 0x20) && (atLineStart || atLineEnd) {
			escape = true
		}
		// '.' at line start would collide with NNTP dot-stuffing; escape it too.
		if !escape && atLineStart && out == 0x2E {
			escape = true
		}

		if escape {
			buf.WriteByte('=')
			buf.WriteByte(byte((int(out) + 64) % 256))
		} else {
			buf.WriteByte(out)
		}
		col++
		if col >= yEncLineLen {
			buf.WriteString("\r\n")
			col = 0
		}
	}
	if col > 0 {
		buf.WriteString("\r\n")
	}

	partCRC := crc32.ChecksumIEEE(data)
	if part > 0 {
		if fileCRC != 0 && part == total {
			fmt.Fprintf(&buf, "=yend size=%d part=%d pcrc32=%08x crc32=%08x\r\n",
				len(data), part, partCRC, fileCRC)
		} else {
			fmt.Fprintf(&buf, "=yend size=%d part=%d pcrc32=%08x\r\n",
				len(data), part, partCRC)
		}
	} else {
		fmt.Fprintf(&buf, "=yend size=%d crc32=%08x\r\n", len(data), partCRC)
	}
	return buf.Bytes()
}

// DecodedPart is the result of decoding a yEnc article body.
type DecodedPart struct {
	Filename  string
	TotalSize int64
	Part      int
	Total     int
	Begin     int64 // 1-based inclusive
	End       int64 // 1-based inclusive
	Data      []byte
	PartCRC   uint32
	FileCRC   uint32
}

// YEncDecode parses a yEnc article body (already un-dot-stuffed).
// The reader is expected to be the article payload between the NNTP "222"
// (or "240") response and the final "." line.
func YEncDecode(r io.Reader) (*DecodedPart, error) {
	br := bufio.NewReader(r)
	out := &DecodedPart{}
	var body bytes.Buffer
	started := false

	for {
		line, err := br.ReadString('\n')
		if err == io.EOF && line == "" {
			break
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case strings.HasPrefix(line, "=ybegin"):
			parseKV(line[len("=ybegin"):], out, false)
			started = true
		case strings.HasPrefix(line, "=ypart"):
			parseKV(line[len("=ypart"):], out, true)
		case strings.HasPrefix(line, "=yend"):
			parseEnd(line[len("=yend"):], out)
			goto done
		default:
			if !started {
				continue
			}
			for i := 0; i < len(line); i++ {
				c := line[i]
				if c == '=' && i+1 < len(line) {
					i++
					body.WriteByte(byte((int(line[i]) - 64 - 42 + 512) % 256))
				} else {
					body.WriteByte(byte((int(c) - 42 + 256) % 256))
				}
			}
		}
		if err == io.EOF {
			break
		}
	}
done:

	out.Data = body.Bytes()
	if !started {
		return nil, errors.New("yenc: =ybegin not found")
	}
	if got := crc32.ChecksumIEEE(out.Data); out.PartCRC != 0 && got != out.PartCRC {
		return out, fmt.Errorf("yenc: pcrc32 mismatch: got %08x want %08x", got, out.PartCRC)
	}
	return out, nil
}

func parseKV(s string, out *DecodedPart, part bool) {
	// Key=value pairs are space separated; values may contain '=' only in
	// name= which is always the last field per yEnc spec.
	s = strings.TrimLeft(s, " ")
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := s[:eq]
		rest := s[eq+1:]
		if key == "name" {
			out.Filename = rest
			break
		}
		sp := strings.IndexByte(rest, ' ')
		var val string
		if sp < 0 {
			val = rest
			s = ""
		} else {
			val = rest[:sp]
			s = strings.TrimLeft(rest[sp:], " ")
		}
		switch key {
		case "size":
			if !part {
				out.TotalSize, _ = strconv.ParseInt(val, 10, 64)
			}
		case "part":
			out.Part, _ = strconv.Atoi(val)
		case "total":
			out.Total, _ = strconv.Atoi(val)
		case "begin":
			out.Begin, _ = strconv.ParseInt(val, 10, 64)
		case "end":
			out.End, _ = strconv.ParseInt(val, 10, 64)
		}
	}
}

func parseEnd(s string, out *DecodedPart) {
	s = strings.TrimLeft(s, " ")
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := s[:eq]
		rest := s[eq+1:]
		sp := strings.IndexByte(rest, ' ')
		var val string
		if sp < 0 {
			val = rest
			s = ""
		} else {
			val = rest[:sp]
			s = strings.TrimLeft(rest[sp:], " ")
		}
		switch key {
		case "pcrc32":
			v, _ := strconv.ParseUint(val, 16, 32)
			out.PartCRC = uint32(v)
		case "crc32":
			v, _ := strconv.ParseUint(val, 16, 32)
			out.FileCRC = uint32(v)
		}
	}
}
