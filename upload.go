package errorcatcher

import (
	"bytes"
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

	if runtime.GOOS == "windows" {
		fmt.Printf("post body size: %d bytes\n", b.Len())
	}

	// Create request
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", w.FormDataContentType())

	if runtime.GOOS == "windows" {
		fmt.Print("sent without gzip")
	}

	return req, nil
}
