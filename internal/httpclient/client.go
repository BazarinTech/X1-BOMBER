package httpclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// Response is the simplified response used by the tool.
type Response struct {
	Status     string
	StatusCode int
	Body       string
}

// Stats contains aggregated results after bulk run.
type Stats struct {
	TotalPerStatus map[int]int    // status -> count
	ExampleMessage map[int]string // sample message/body for that status
}

// SendRequest sends a single HTTP request using provided client.
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

// createClient optionally creates HTTP client that uses SOCKS5 at 127.0.0.1:9050 when useTor==true.
func createClient(useTor bool) (*http.Client, error) {
	if !useTor {
		return &http.Client{Timeout: 20 * time.Second}, nil
	}

	// attempt to create a SOCKS5 dialer to Tor
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("creating socks5 dialer: %w", err)
	}
	// Build a transport that uses the socks dialer
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	transport := &http.Transport{
		DialContext: dialContext,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// SendMultipleRequests concurrently sends `count` requests. payloadGen(i) returns a map[field]value
// which will be converted into a JSON-like body: {"Field1":"value1","Field2":"value2"}.
// If you need custom formatting change here.
func SendMultipleRequests(method, url string, headers map[string]string, payloadGen func(int) map[string]string, count, concurrency int, useTor bool) (*Stats, error) {
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

	// results aggregation
	stats := &Stats{
		TotalPerStatus: map[int]int{},
		ExampleMessage: map[int]string{},
	}
	var mu sync.Mutex

	type job struct {
		idx int
	}
	jobs := make(chan job, count)
	results := make(chan *Response, count)
	errorsCh := make(chan error, count)

	// worker
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for j := range jobs {
			payloadMap := payloadGen(j.idx)
			// create body as JSON-like map string
			// Build a simple JSON body (no escaping implemented for brevity; if your data contains quotes, refine this)
			bodyBuf := &bytes.Buffer{}
			bodyBuf.WriteString("{")
			first := true
			for k, v := range payloadMap {
				if !first {
					bodyBuf.WriteString(",")
				}
				fmt.Fprintf(bodyBuf, `"%s":"%s"`, k, v)
				first = false
			}
			bodyBuf.WriteString("}")

			resp, err := SendRequestWithClient(client, method, url, bodyBuf.Bytes(), headers)
			if err != nil {
				errorsCh <- err
				continue
			}
			results <- resp
		}
	}

	// start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	// enqueue jobs
	go func() {
		for i := 0; i < count; i++ {
			jobs <- job{idx: i}
		}
		close(jobs)
	}()

	// collect
	go func() {
		wg.Wait()
		close(results)
		close(errorsCh)
	}()

	// read responses and errors
	processed := 0
	for processed < count {
		select {
		case r, ok := <-results:
			if !ok {
				// results closed; drain errors
				for err := range errorsCh {
					_ = err
				}
				processed = count
				break
			}
			mu.Lock()
			stats.TotalPerStatus[r.StatusCode]++
			if _, exists := stats.ExampleMessage[r.StatusCode]; !exists {
				// store first body as example message (trim to 120 chars)
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
			// treat network errors as status 0
			mu.Lock()
			stats.TotalPerStatus[0]++
			if _, exists := stats.ExampleMessage[0]; !exists {
				stats.ExampleMessage[0] = err.Error()
			}
			mu.Unlock()
			processed++
		case <-time.After(1 * time.Second):
			// prevent blocking if both channels are empty; continue until processed reaches count
		}
	}

	return stats, nil
}

// InstallTorProxychains attempts to install tor and proxychains (Debian-based).
func InstallTorProxychains() error {
	// basic check: is apt-get present?
	_, err := exec.LookPath("apt-get")
	if err != nil {
		return fmt.Errorf("apt-get not found; automatic install only supported on apt-based systems")
	}
	// run update + install (requires sudo)
	cmd := exec.Command("bash", "-lc", "sudo apt-get update && sudo apt-get install -y tor proxychains")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install command failed: %v\noutput: %s", err, string(out))
	}
	return nil
}
