package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/AtOnline/drive-webdav/oauth2"
)

type fsNodeFile struct {
	self *fsNode
	flag int
	perm os.FileMode

	// specific to uploads
	parent *fsNode
	upload *oauth2.Upload

	pos int64

	resp *http.Response
	rpos int64 // pos in response
}

func (f *fsNodeFile) Close() error {
	f.pos = 0
	return f.finalizeUpload()
}

func (f *fsNodeFile) finalizeUpload() error {
	if f.upload != nil {
		final, err := f.upload.Complete()
		if err != nil {
			return err
		}
		f.upload = nil

		// add child if new upload
		if f.self == nil {
			f.parent.load()
			f.self = f.parent.addChild(final.Data.(map[string]interface{}), "")
		} else {
			f.self.store(final.Data.(map[string]interface{}))
		}
	}
	return nil
}

func (f *fsNodeFile) Read(d []byte) (int, error) {
	if f.flag&os.O_RDONLY != os.O_RDONLY && f.flag&os.O_RDWR != os.O_RDWR {
		return 0, os.ErrInvalid
	}

	// perform a read, intelligently (ha ha)
	if f.resp != nil {
		if f.pos > f.rpos && f.pos < (f.rpos+8*1024) {
			// we can read less than 8k of data to reach pos, that's probably faster than establishing a new http request
			drop := f.pos - f.rpos
			n, err := f.resp.Body.Read(make([]byte, drop))
			if n >= 0 {
				// with that, f.rpos should be == f.pos
				f.rpos += int64(n)
			}
			if err != nil {
				return 0, err
			}
		}
		// can we use this response?
		if f.rpos == f.pos {
			// yes.
			n, err := f.resp.Body.Read(d)
			if n > 0 {
				f.rpos += int64(n)
				f.pos += int64(n)
			}
			return n, err
		}

		// cannot use this response
		f.resp.Body.Close()
		f.resp = nil
	}

	if f.pos < 0 {
		// jsut in case, sanity check
		return 0, errors.New("negative seek not supported")
	}
	if f.pos >= f.self.size {
		// out of file
		return 0, io.EOF
	}

	if f.self.url == "" {
		// just do nothing since webdav doesn't like errors
		return len(d), nil
		//return 0, os.ErrPermission
	}

	req, err := http.NewRequest("GET", f.self.url, nil)
	if err != nil {
		return 0, err
	}

	if f.pos != 0 {
		// need to add range to request headers
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", f.pos))
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}

	f.resp = res
	f.rpos = f.pos

	// perform read
	n, err := f.resp.Body.Read(d)
	if n > 0 {
		f.rpos += int64(n)
		f.pos += int64(n)
	}
	return n, err
}

func (f *fsNodeFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}

func (f *fsNodeFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
		return f.pos, nil
	case io.SeekCurrent:
		f.pos += offset
		return f.pos, nil
	case io.SeekEnd:
		f.pos = f.self.size + offset
		return f.pos, nil
	default:
		return f.pos, os.ErrInvalid
	}
}

func (f *fsNodeFile) Stat() (os.FileInfo, error) {
	err := f.finalizeUpload()
	if err != nil {
		return nil, err
	}
	if f.self == nil {
		return nil, os.ErrNotExist
	}
	return f.self, nil
}

func (f *fsNodeFile) Write(d []byte) (int, error) {
	if f.upload == nil {
		// TODO check if write access
		var err error
		f.upload, err = f.self.overwrite()
		if err != nil {
			return 0, err
		}
	}
	if f.pos != f.upload.Len() {
		// can't write here
		return 0, os.ErrInvalid
	}
	n, err := f.upload.Write(d)
	if n > 0 {
		f.pos += int64(n)
	}
	return n, err
}
