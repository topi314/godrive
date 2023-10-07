package esbuild

import (
	"io/fs"
	"path"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

var (
	_ fs.FS       = (*esbuildFS)(nil)
	_ fs.File     = (*outputFile)(nil)
	_ fs.FileInfo = (*outputFileInfo)(nil)
)

func FS(result api.BuildResult) fs.FS {
	return &esbuildFS{
		Files: result.OutputFiles,
	}
}

type esbuildFS struct {
	Files []api.OutputFile
}

func (b esbuildFS) Open(name string) (fs.File, error) {
	for _, file := range b.Files {
		if file.Path == name {
			return (*outputFile)(&file), nil
		}
	}
	return nil, fs.ErrNotExist
}

type outputFile api.OutputFile

func (o *outputFile) Stat() (fs.FileInfo, error) {
	return &outputFileInfo{o}, nil
}

func (o *outputFile) Read(bytes []byte) (int, error) {
	return copy(bytes, o.Contents), nil
}

func (o *outputFile) Close() error {
	return nil
}

type outputFileInfo struct {
	*outputFile
}

func (o *outputFileInfo) Name() string {
	return path.Base(o.outputFile.Path)
}

func (o *outputFileInfo) Size() int64 {
	return int64(len(o.outputFile.Contents))
}

func (o *outputFileInfo) Mode() fs.FileMode {
	return 0
}

func (o *outputFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (o *outputFileInfo) IsDir() bool {
	return false
}

func (o *outputFileInfo) Sys() any {
	return nil
}
