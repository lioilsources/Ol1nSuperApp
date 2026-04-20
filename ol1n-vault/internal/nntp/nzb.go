package nntp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

const nzbNamespace = "http://www.newzbin.com/DTD/2003/nzb"
const nzbDoctype = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.1//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.1.dtd">
`

type NZB struct {
	XMLName xml.Name  `xml:"nzb"`
	Xmlns   string    `xml:"xmlns,attr"`
	Files   []NZBFile `xml:"file"`
}

type NZBFile struct {
	Poster   string       `xml:"poster,attr"`
	Date     int64        `xml:"date,attr"`
	Subject  string       `xml:"subject,attr"`
	Groups   NZBGroups    `xml:"groups"`
	Segments NZBSegments  `xml:"segments"`
}

type NZBGroups struct {
	Group []string `xml:"group"`
}

type NZBSegments struct {
	Segment []NZBSegment `xml:"segment"`
}

type NZBSegment struct {
	Bytes   int    `xml:"bytes,attr"`
	Number  int    `xml:"number,attr"`
	MsgID   string `xml:",chardata"`
}

// BuildNZB constructs a minimal NZB document referencing a single uploaded file.
// msgIDs should not include angle brackets.
func BuildNZB(filename, newsgroup, poster string, totalParts int, segments []NZBSegment) ([]byte, error) {
	subject := fmt.Sprintf("[1/1] %s yEnc (1/%d)", filename, totalParts)
	doc := NZB{
		Xmlns: nzbNamespace,
		Files: []NZBFile{{
			Poster:  poster,
			Date:    time.Now().Unix(),
			Subject: subject,
			Groups:  NZBGroups{Group: []string{newsgroup}},
			Segments: NZBSegments{Segment: segments},
		}},
	}
	var buf bytes.Buffer
	buf.WriteString(nzbDoctype)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", " ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// ParseNZB decodes an NZB XML document.
func ParseNZB(r io.Reader) (*NZB, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var n NZB
	dec := xml.NewDecoder(bytes.NewReader(raw))
	// Some NZB files use DOCTYPE declarations; encoding/xml tolerates those.
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("nzb: decode: %w", err)
	}
	// Trim angle brackets that may have been included in segment bodies.
	for i := range n.Files {
		for j := range n.Files[i].Segments.Segment {
			seg := &n.Files[i].Segments.Segment[j]
			seg.MsgID = strings.TrimSpace(seg.MsgID)
			seg.MsgID = strings.TrimPrefix(seg.MsgID, "<")
			seg.MsgID = strings.TrimSuffix(seg.MsgID, ">")
		}
	}
	return &n, nil
}
