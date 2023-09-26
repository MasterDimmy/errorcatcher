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
	var err error
	w := multipart.NewWriter(&b)
	for key, r := range values {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
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

	}
	w.Close()

	//zip it
	buff := &bytes.Buffer{}
	wr := gzip.NewWriter(buff)
	wr.Header.Name = "body"
	_, err = wr.Write(b.Bytes())
	if err != nil {
		return nil, err
	}
	wr.Close()

	if runtime.GOOS == "windows" {
		fmt.Printf("post body size: %d bytes\n", buff.Len())
	}

	//request
	req, err := http.NewRequest("POST", url, buff)
	if err != nil {
		return nil, err
	}
	//req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Boundary", w.Boundary())
	return req, nil
}
