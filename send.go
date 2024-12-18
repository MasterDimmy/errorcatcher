package errorcatcher

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MasterDimmy/golang-lruexpire"
)

type taskData struct {
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

	TurnOff bool

	exename string //name of the executable

	once    sync.Once
	tasks   chan *taskData
	working int64

	resendM                       sync.Mutex
	ResendNotSended               bool    //do resend if first try fails?
	ResendMaxStorageSizeInMB      float64 //default=0, 100MB
	ResendMaxStorageDurationHours int     //hours, default=24*31 = 1 month
	ResendStorage                 string  // "./errcatcherresender/"
}

// send error message
func (s *System) Send(text string) {
	if s.TurnOff {
		return
	}

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

	atomic.AddInt64(&s.working, 1)
	s.tasks <- &taskData{text: text}
}

// send error message with file
func (s *System) SendWithFile(text string, filenames []string) {
	if s.TurnOff {
		return
	}

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

	atomic.AddInt64(&s.working, 1)
	s.tasks <- &taskData{text: text, fname: filenames}
}

// ensure all tasks sent
func (s *System) Wait() {
	for atomic.LoadInt64(&s.working) > 0 {
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *System) sender() {
	s.once.Do(func() {
		if s.ResendNotSended {
			os.MkdirAll(s.ResendStorage, 0755) // Ensure proper permissions

			s.setDefaults()
			go s.resend()
		}

		s.tasks = make(chan *taskData, 1000)

		host, _ := os.Hostname()
		s.exename, _ = os.Executable()
		s.exename = host + " - " + path.Base(s.exename)

		textAndFiles, _ := lru.NewARCWithExpire(100, time.Minute) // Skip same messages being sent

		go func() {
			for {
				select {
				case msg := <-s.tasks:
					s.processMessage(msg, textAndFiles)
				case <-time.After(100 * time.Millisecond):
					// Sleep to reduce CPU usage
				}
			}
		}()
	})
}

func (s *System) processMessage(msg *taskData, textAndFiles *lru.ARCCache) {
	defer atomic.AddInt64(&s.working, -1)

	textAndFiles.Add(msg.text, 1)
	for _, v := range msg.fname {
		textAndFiles.Add("f"+v, 1)
	}

	mx := len(msg.text)
	if mx > 2000 {
		msg.text = msg.text[:2000]
	}

	atomic.AddInt64(&s.working, 1) // this and in task
	defer atomic.AddInt64(&s.working, -1)

	ok := true
	thisNum := 2
	for ok {
		select {
		case t := <-s.tasks:
			defer atomic.AddInt64(&s.working, -1)

			ok := textAndFiles.Contains(t.text)
			ok2 := true
			for _, v := range t.fname {
				ok3 := textAndFiles.Contains("f" + v)
				if !ok3 {
					ok2 = false
					break
				}
			}

			if !ok || !ok2 {
				textAndFiles.Add(t.text, 1)
				for _, v := range t.fname {
					textAndFiles.Add("f"+v, 1)
				}

				mx := len(t.text)
				if mx > 2000 {
					t.text = t.text[:2000]
				}

				msg.text += fmt.Sprintf("\n------------- MSG %d , %s -------------\n", thisNum, formatTime(time.Now().Unix())) + t.text
				msg.fname = append(msg.fname, t.fname...)
				thisNum++
			}
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

	filesCnt := 0
	for _, v := range msg.fname {
		if len(v) > 1 {
			f, err := os.Open(v)
			if err != nil {
				fmt.Printf("errorcatcher: %s => %s ", v, err.Error())
				continue
			}
			defer f.Close()

			mdata["file"+fmt.Sprintf("%d", filesCnt)] = f
			mdata["fname"+fmt.Sprintf("%d", filesCnt)] = bytes.NewBufferString(v)
			filesCnt++
		}
	}

	if runtime.GOOS == "windows" {
		fmt.Printf("do request: %+v , files: %d \n", mdata["data"], filesCnt)
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
		fmt.Println("errorcatcher: response is nil")
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
}

func (s *System) addForResend(req *http.Request) {
	s.resendM.Lock()
	defer s.resendM.Unlock()
	atomic.AddInt64(&s.working, 1)
	defer atomic.AddInt64(&s.working, -1)

	// Dump the request to a byte slice
	data, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}

	f, err := os.Create(s.ResendStorage + fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63()) + ".rsnd")
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}
	wr := gzip.NewWriter(f)
	_, err = wr.Write(data)
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}
	err = wr.Close()
	if err != nil {
		fmt.Println("ERROR: ", err)
		return
	}
}

type fileWithTime struct {
	name string
	mod  time.Time
	size int64
}

// resend cycle for not resent files
func (s *System) resend() {
	for {
		func() {
			s.resendM.Lock()
			defer s.resendM.Unlock()

			//scan files to send
			files, err := ioutil.ReadDir(s.ResendStorage)
			if err != nil {
				log.Printf("Error reading directory: %v\n", err)
				return
			}

			var filesWithTime []fileWithTime

			//collect filenames and it's creation dates
			totalSize := int64(0)

			for _, file := range files {
				if !file.IsDir() { // Ignore directories
					filesWithTime = append(filesWithTime, fileWithTime{
						name: file.Name(),
						mod:  file.ModTime(),
						size: file.Size(),
					})
					totalSize += file.Size()
				}
			}

			// Sort files by creation time
			sort.Slice(filesWithTime, func(i, j int) bool {
				return filesWithTime[i].mod.Before(filesWithTime[j].mod)
			})

			//remove old (overmax count or size) to allow store new
			for i, fwt := range filesWithTime {
				if float64(totalSize) > float64(s.ResendMaxStorageSizeInMB*1024*1024) ||
					time.Now().After(fwt.mod.Add(time.Duration(s.ResendMaxStorageDurationHours)*time.Hour)) {
					fmt.Printf("Removed: Filename: %s, Creation Time: %v Size: %d (total:%d)\n", fwt.name, fwt.mod, fwt.size, totalSize)
					totalSize -= fwt.size
					filesWithTime[i].size = 0 //mark as removed
					os.Remove(s.ResendStorage + fwt.name)
				}
			}

			//send files starting from last
			for _, fwt := range filesWithTime {
				if fwt.size > 0 {
					if !s.sendStoredRequest(s.ResendStorage + fwt.name) {
						fmt.Printf("cant send: %s\n", fwt.name)
					} else {
						//sent ok
						os.Remove(s.ResendStorage + fwt.name)
					}
				}
			}

		}()

		time.Sleep(time.Minute * 5)
	}
}

// defaults
func (s *System) setDefaults() {
	s.resendM.Lock()
	defer s.resendM.Unlock()

	if s.ResendMaxStorageSizeInMB == 0 {
		s.ResendMaxStorageSizeInMB = 100 //default=0, 100MB
	}
	if s.ResendMaxStorageDurationHours == 0 {
		s.ResendMaxStorageDurationHours = 24 * 31 //hours, default=24*31 = 1 month
	}

	if s.ResendStorage == "" {
		s.ResendStorage = "./errcatcherresender/"
	}
}

func (s *System) sendStoredRequest(name string) (ret bool) {
	// Read the dumped request from the file
	data, err := ioutil.ReadFile(name)
	if err != nil {
		fmt.Printf("cant read file: %s: %s", name, err.Error())
		return
	}

	// Convert the byte slice back to an http.Request object
	buffer := bytes.NewBuffer(data)
	reader := ioutil.NopCloser(buffer)

	gz, err := gzip.NewReader(reader)
	if err != nil {
		fmt.Printf("cant ungzip stored: %s\n", err.Error())
		return
	}
	defer gz.Close()

	req, err := http.ReadRequest(bufio.NewReader(gz))
	if err != nil {
		fmt.Printf("cant read request: %s", err.Error())
		return
	}

	//request
	newreq, err := http.NewRequest("POST", s.CollectorUrl, req.Body)
	if err != nil {
		fmt.Printf("cant create request: %s", err.Error())
		return
	}
	newreq.Header.Set("Content-Encoding", "gzip")
	newreq.Header.Set("Boundary", req.Header.Get("Boundary"))

	// Create an HTTP client and send the request
	client := &http.Client{}
	resp, err := client.Do(newreq)
	if err != nil {
		fmt.Printf("cant client do: %s\n", err.Error())
		return
	}
	defer resp.Body.Close()

	// Print the response for demonstration purposes
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 || len(data) == 0 || string(data) != "OK" {
		if err != nil {
			fmt.Printf("cant readall: %s\n", err.Error())
			return
		}
	}

	ret = true
	return
}
