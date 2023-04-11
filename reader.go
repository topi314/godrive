package main

import (
	"io"
	"os"
	"path"
)

type eofReader struct{}

func (r eofReader) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (r eofReader) Close() error {
	return nil
}

type folderReader struct {
	filenames []string
	sep       []byte

	cur     io.ReadCloser
	sepLeft int
}

// newFolderReader reads from a directory, and lazily loads files as it needs it.
// It is a reader that reads a concatenation of those files separated by the separator.
// You must call Close to close the last file in the list.
func newFolderReader(directory string) (*folderReader, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		filenames = append(filenames, path.Join(directory, info.Name()))
	}
	var cur io.ReadCloser
	if len(entries) > 0 {
		var filename string
		filename, filenames = filenames[0], filenames[1:]

		var err error
		if cur, err = os.Open(filename); err != nil {
			return nil, err
		}
	} else {
		cur = eofReader{}
	}
	return &folderReader{filenames, []byte("\n"), cur, 0}, nil
}

func (r *folderReader) Read(p []byte) (int, error) {
	m := r.writeSep(p)
	n, err := r.cur.Read(p[m:])
	n += m

	// current reader is finished, load in the new reader
	if err == io.EOF && len(r.filenames) > 0 {
		if err := r.cur.Close(); err != nil {
			return n, err
		}

		var filename string
		filename, r.filenames = r.filenames[0], r.filenames[1:]
		if r.cur, err = os.Open(filename); err != nil {
			return n, err
		}
		r.sepLeft = len(r.sep)

		// if previous read returned (0, io.EOF), read from the new reader
		if n == 0 {
			return r.Read(p)
		}
		n += r.writeSep(p[n:])
	}
	return n, err
}

func (r *folderReader) writeSep(p []byte) int {
	m := 0
	if r.sepLeft > 0 {
		m = copy(p, r.sep[len(r.sep)-r.sepLeft:])
		r.sepLeft -= m
	}
	return m
}

func (r *folderReader) Close() error {
	return r.cur.Close()
}
