package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type Response struct {
	Status     string
	StatusCode int
	Body       string
}

type Stats struct {
	TotalPerStatus map[int]int
	ExampleMessage map[int]string
}

func SendRequestWithClient(client *http.Client, method, url string, body []byte, headers map[string]string) (*Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return &Response{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Body:       string(b),
	}, nil
}

func createClient(useTor bool) (*http.Client, error) {
	if !useTor {
		return &http.Client{Timeout: 20 * time.Second}, nil
	}
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("creating socks5 dialer: %w", err)
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	transport := &http.Transport{DialContext: dialContext}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func SendMultipleRequests(
	method, url string,
	headers map[string]string,
	payloadGen func(int) map[string]string,
	count, concurrency int,
	useTor bool,
	payloadType string,
) (*Stats, error) {

	if count <= 0 {
		return nil, fmt.Errorf("invalid count")
	}
	if concurrency <= 0 {
		concurrency = 10
	}

	client, err := createClient(useTor)
	if err != nil {
		return nil, err
	}

	stats := &Stats{TotalPerStatus: map[int]int{}, ExampleMessage: map[int]string{}}
	var mu sync.Mutex
	type job struct{ idx int }
	jobs := make(chan job, count)
	results := make(chan *Response, count)
	errorsCh := make(chan error, count)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			payloadMap := payloadGen(j.idx)
			var bodyBytes []byte
			var contentType string

			switch payloadType {
			case "json":
				buf := &bytes.Buffer{}
				buf.WriteString("{")
				first := true
				for k, v := range payloadMap {
					if !first {
						buf.WriteString(",")
					}
					fmt.Fprintf(buf, `"%s":"%s"`, k, v)
					first = false
				}
				buf.WriteString("}")
				bodyBytes = buf.Bytes()
				contentType = "application/json"

			case "form":
				form := make([]string, 0, len(payloadMap))
				for k, v := range payloadMap {
					form = append(form, fmt.Sprintf("%s=%s", k, v))
				}
				bodyBytes = []byte(strings.Join(form, "&"))
				contentType = "application/x-www-form-urlencoded"

			case "multipart":
				var b bytes.Buffer
				w := multipart.NewWriter(&b)
				for k, v := range payloadMap {
					_ = w.WriteField(k, v)
				}
				w.Close()
				bodyBytes = b.Bytes()
				contentType = w.FormDataContentType()

			case "binary":
				for _, v := range payloadMap {
					bodyBytes = []byte(v)
					break
				}
				contentType = "application/octet-stream"

			case "graphql":
				query := payloadMap["query"]
				bodyBytes = []byte(fmt.Sprintf(`{"query":"%s"}`, query))
				contentType = "application/json"

			default:
				return
			}

			headers["Content-Type"] = contentType

			resp, err := SendRequestWithClient(client, method, url, bodyBytes, headers)
			if err != nil {
				errorsCh <- err
				continue
			}
			results <- resp
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		for i := 0; i < count; i++ {
			jobs <- job{idx: i}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
		close(errorsCh)
	}()

	processed := 0
	for processed < count {
		select {
		case r, ok := <-results:
			if !ok {
				for err := range errorsCh {
					_ = err
				}
				processed = count
				break
			}
			mu.Lock()
			stats.TotalPerStatus[r.StatusCode]++
			if _, exists := stats.ExampleMessage[r.StatusCode]; !exists {
				msg := r.Body
				if len(msg) > 120 {
					msg = msg[:120] + "..."
				}
				stats.ExampleMessage[r.StatusCode] = msg
			}
			mu.Unlock()
			processed++
		case err, ok := <-errorsCh:
			if !ok {
				processed = count
				break
			}
			mu.Lock()
			stats.TotalPerStatus[0]++
			if _, exists := stats.ExampleMessage[0]; !exists {
				stats.ExampleMessage[0] = err.Error()
			}
			mu.Unlock()
			processed++
		case <-time.After(1 * time.Second):
		}
	}

	return stats, nil
}

func InstallTorProxychains() error {
	_, err := exec.LookPath("apt-get")
	if err != nil {
		return fmt.Errorf("apt-get not found; automatic install only supported on apt-based systems")
	}
	cmd := exec.Command("bash", "-lc", "sudo apt-get update && sudo apt-get install -y tor proxychains")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install command failed: %v\noutput: %s", err, string(out))
	}
	return nil
}
