package errorcatcher

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"runtime"
)

func uploadRequest(url string, values map[string]io.Reader) (*http.Request, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for key, r := range values {
		var fw io.Writer
		var err error

		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return nil, err
			}
		} else {
			if fw, err = w.CreateFormField(key); err != nil {
				return nil, err
			}
		}

		if _, err = io.Copy(fw, r); err != nil {
			return nil, err
		}

		if c, ok := r.(io.Closer); ok {
			if err := c.Close(); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	// Zip it
	buff := &bytes.Buffer{}
	wr := gzip.NewWriter(buff)
	wr.Name = "body"

	if _, err := wr.Write(b.Bytes()); err != nil {
		return nil, err
	}

	if err := wr.Close(); err != nil {
		return nil, err
	}

	if runtime.GOOS == "windows" {
		fmt.Printf("post body size: %d bytes\n", buff.Len())
	}

	// Create request
	req, err := http.NewRequest("POST", url, buff)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", w.FormDataContentType())

	return req, nil
}
