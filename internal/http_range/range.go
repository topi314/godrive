package http_range

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidRangeHeaderStart = errors.New("invalid range header, must start with 'bytes='")
	ErrInvalidRangeHeader      = func(header string) error {
		return fmt.Errorf("invalid range header: %s", header)
	}
)

type Range struct {
	Start int64
	End   int64
	Size  int64
}

func (r *Range) String() string {
	if r.End == 0 {
		return fmt.Sprintf("bytes %d-%d/%d", r.Start, r.Size-1, r.Size)
	}
	return fmt.Sprintf("bytes %d-%d/%d", r.Start, r.End, r.Size)
}

func (r *Range) Limit() int64 {
	if r.End == 0 {
		return r.Size - r.Start
	}

	return r.End - r.Start + 1
}

func ParseRange(header string, size int64) (*Range, error) {
	if header == "" {
		return nil, nil
	}

	if !strings.HasPrefix(header, "bytes=") {
		return nil, ErrInvalidRangeHeaderStart
	}

	var (
		start int64
		end   int64
	)
	if _, err := fmt.Sscanf(header, "bytes=%d-%d", &start, &end); err == nil {
		return &Range{Start: start, End: end, Size: size}, nil
	}
	if _, err := fmt.Sscanf(header, "bytes=%d-", &start); err == nil {
		return &Range{Start: start, End: size - 1, Size: size}, nil
	}
	if _, err := fmt.Sscanf(header, "bytes=-%d", &end); err == nil {
		return &Range{End: size - end, Size: size}, nil
	}

	return nil, ErrInvalidRangeHeader(header)
}
