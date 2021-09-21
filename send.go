package errorcatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	//"github.com/valyala/fasthttp"
)

type task_data struct {
	text  string
	fname []string
}

type TCatchedError struct {
	Id    int64
	Name  string
	Exe   string
	Text  string
	When  int64
	Nicks string //Nick via ;
}

type System struct {
	Name string   //system name
	Nick []string //whom to inform in chat ?

	CollectorUrl string //chat connector url

	exename string //name of the executable

	once    sync.Once
	tasks   chan *task_data
	working int64
}

//send error message
func (s *System) Send(text string) {
	s.sender()

	if len(s.Name) == 0 {
		panic("errorcatcher: system name cant be empty")
	}

	if len(s.Nick) == 0 {
		panic("errorcatcher: nick to receive message cant be empty")
	}

	if len(s.CollectorUrl) == 0 {
		panic("errorcatcher: CollectorUrl is nil")
	}

	s.tasks <- &task_data{text: text}
}

//send error message
func (s *System) SendWithFile(text string, filenames []string) {
	s.sender()

	if len(s.Name) == 0 {
		panic("errorcatcher: system name cant be empty")
	}

	if len(s.Nick) == 0 {
		panic("errorcatcher: nick to receive message cant be empty")
	}

	if len(s.CollectorUrl) == 0 {
		panic("errorcatcher: CollectorUrl is nil")
	}

	for _, v := range filenames {
		if len(v) == 0 {
			panic("errorcatcher: file name cant be empty")
		}
	}

	s.tasks <- &task_data{text: text, fname: filenames}
}

//ensure all tasks sent
func (s *System) Wait() {
	for atomic.LoadInt64(&s.working) == 1 {
		time.Sleep(time.Second)
	}
}

func (s *System) sender() {
	s.once.Do(func() {
		s.tasks = make(chan *task_data, 1000)

		host, _ := os.Hostname()
		s.exename, _ = os.Executable()
		s.exename = host + " - " + path.Base(s.exename)

		go func() {
			for {
				func() {
					msg := <-s.tasks
					mx := len(msg.text)
					if mx > 1000 {
						msg.text = msg.text[:1000]
					}
					msg.text += "\n"
					atomic.StoreInt64(&s.working, 1)
					defer atomic.StoreInt64(&s.working, 0)

					ok := true
					for ok { //???????? ????????? ?????????
						select {
						case t := <-s.tasks:
							mx := len(t.text)
							if mx > 1000 {
								t.text = t.text[:1000]
							}
							msg.text += t.text + "\n" //????? ???????????
							msg.fname = append(msg.fname, t.fname...)
						default:
							ok = false
						}
					}

					buf, _ := json.Marshal(&TCatchedError{
						Name:  s.Name,
						Exe:   s.exename,
						Text:  msg.text,
						When:  time.Now().Unix(),
						Nicks: strings.Join(s.Nick, ";"),
					})

					bb := bytes.NewBuffer(buf)
					mdata := make(map[string]io.Reader)
					mdata["data"] = bb

					for i, v := range msg.fname {
						if len(v) > 1 {
							f, err := os.Open(v)
							if err != nil {
								fmt.Printf("errorcatcher: %s => %s ", v, err.Error())
								return
							}
							defer f.Close()
							mdata["file"+fmt.Sprintf("%d", i)] = f
						}
					}

					if runtime.GOOS == "windows" {
						fmt.Printf("do request: %+v\n", mdata)
					}

					req, err := uploadRequest(s.CollectorUrl, mdata)
					if err != nil {
						fmt.Println("errorcatcher: " + err.Error())
						return
					}

					client := &http.Client{
						Timeout: time.Minute,
					}
					resp, err := client.Do(req)
					if err != nil {
						fmt.Println("errorcatcher: " + err.Error())
						return
					}
					if resp == nil {
						fmt.Println("errorcatcher: responce is nil")
						return
					}
					data, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						fmt.Println("errorcatcher: " + err.Error())
						return
					}
					defer resp.Body.Close()
					c := resp.StatusCode

					if err != nil || c != 200 || len(data) == 0 || string(data) != "OK" {
						if err != nil {
							fmt.Println("errorcatcher: " + err.Error())
							return
						}
					}
					if runtime.GOOS == "windows" {
						fmt.Printf("sent catched error\ncode:%d  body: %s\n", c, string(data))
					}
				}()
			}
		}()
	})
}

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
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}
