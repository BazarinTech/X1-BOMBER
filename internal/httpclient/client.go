package httpclient

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
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

// SendRequestWithClient sends a single HTTP request using provided client.
// It streams a small snippet of the response body and discards the rest to avoid large memory usage.
func SendRequestWithClient(client *http.Client, method, urlStr string, body []byte, headers map[string]string) (*Response, error) {
	req, err := http.NewRequest(method, urlStr, bytes.NewBuffer(body))
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

	// Read a small snippet (e.g. 512 bytes) and discard the rest.
	const maxSnippet = 512
	limitReader := io.LimitReader(resp.Body, maxSnippet)
	snippet, _ := io.ReadAll(limitReader)
	// drain the remainder (non-blocking)
	_, _ = io.Copy(io.Discard, resp.Body)

	return &Response{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(snippet)),
	}, nil
}

// createClient optionally creates HTTP client that uses SOCKS5 at 127.0.0.1:9050 when useTor==true.
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
	transport := &http.Transport{
		DialContext: dialContext,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// SendMultipleRequests sends many requests in chunked batches, streams responses, and optionally rate-limits and logs.
func SendMultipleRequests(
	method, urlStr string,
	headers map[string]string,
	payloadGen func(int) map[string]string,
	count, concurrency int,
	useTor bool,
	payloadType string,
	chunkSize int,
	rateLimit int, // requests per second, 0 = unlimited
	logPath string, // empty = no per-request logging
) (*Stats, error) {

	if count <= 0 {
		return nil, fmt.Errorf("invalid count")
	}
	if concurrency <= 0 {
		concurrency = 10
	}
	if chunkSize <= 0 {
		chunkSize = 1000
	}

	client, err := createClient(useTor)
	if err != nil {
		return nil, err
	}

	// Prepare logging if requested
	var csvFile *os.File
	var csvWriter *csv.Writer
	logging := false
	if logPath != "" {
		f, err := os.Create(logPath)
		if err != nil {
			return nil, fmt.Errorf("cannot create log file: %w", err)
		}
		csvFile = f
		csvWriter = csv.NewWriter(f)
		// header: index,status,bodySnippet
		_ = csvWriter.Write([]string{"index", "status", "snippet"})
		csvWriter.Flush()
		logging = true
	}

	stats := &Stats{
		TotalPerStatus: map[int]int{},
		ExampleMessage: map[int]string{},
	}
	var globalMu sync.Mutex

	// rate limiter ticker (shared)
	var rateTicker *time.Ticker
	var rateTickC <-chan time.Time
	if rateLimit > 0 {
		interval := time.Duration(float64(time.Second) / float64(rateLimit))
		if interval <= 0 {
			interval = time.Millisecond
		}
		rateTicker = time.NewTicker(interval)
		rateTickC = rateTicker.C
		defer rateTicker.Stop()
	}

	// process in chunks to avoid huge memory for jobs channel
	for start := 0; start < count; start += chunkSize {
		end := start + chunkSize
		if end > count {
			end = count
		}
		batchSize := end - start

		// per-batch channels (bounded to batchSize)
		jobs := make(chan int, batchSize)
		results := make(chan *Response, batchSize)
		errorsCh := make(chan error, batchSize)

		var wg sync.WaitGroup

		worker := func() {
			defer wg.Done()
			for idx := range jobs {
				// rate limiting: wait for ticker if enabled
				if rateLimit > 0 {
					<-rateTickC
				}

				// generate payload for global index idx
				payloadMap := payloadGen(idx)

				// build body according to payloadType
				var bodyBytes []byte
				var contentType string
				switch payloadType {
				case "json":
					// use encoding/json to ensure proper escaping
					jb, err := json.Marshal(payloadMap)
					if err != nil {
						errorsCh <- fmt.Errorf("json marshal: %w", err)
						continue
					}
					bodyBytes = jb
					contentType = "application/json"

				case "form":
					form := url.Values{}
					for k, v := range payloadMap {
						form.Set(k, v)
					}
					bodyBytes = []byte(form.Encode())
					contentType = "application/x-www-form-urlencoded"

				case "multipart":
					var buf bytes.Buffer
					w := multipart.NewWriter(&buf)
					for k, v := range payloadMap {
						// Add as form field. If you'd like to support file upload (value = @/path/file),
						// we can add special handling here.
						_ = w.WriteField(k, v)
					}
					_ = w.Close()
					bodyBytes = buf.Bytes()
					contentType = w.FormDataContentType()

				case "binary":
					// take first value (as before) and send raw bytes
					var v string
					for _, vv := range payloadMap {
						v = vv
						break
					}
					bodyBytes = []byte(v)
					contentType = "application/octet-stream"

				case "graphql":
					// expect a field named "query" containing GraphQL query text
					query := payloadMap["query"]
					g := map[string]interface{}{"query": query}
					jb, err := json.Marshal(g)
					if err != nil {
						errorsCh <- fmt.Errorf("graphql marshal: %w", err)
						continue
					}
					bodyBytes = jb
					contentType = "application/json"

				default:
					errorsCh <- fmt.Errorf("unsupported payload type: %s", payloadType)
					continue
				}

				// Respect user headers but override Content-Type for correctness
				reqHeaders := map[string]string{}
				for k, v := range headers {
					reqHeaders[k] = v
				}
				reqHeaders["Content-Type"] = contentType

				resp, err := SendRequestWithClient(client, method, urlStr, bodyBytes, reqHeaders)
				if err != nil {
					errorsCh <- fmt.Errorf("idx %d: %w", idx, err)
					continue
				}
				results <- resp

				// Optionally log immediately (thread-safe)
				if logging {
					globalMu.Lock()
					_ = csvWriter.Write([]string{fmt.Sprintf("%d", idx), fmt.Sprintf("%d", resp.StatusCode), resp.Body})
					csvWriter.Flush()
					globalMu.Unlock()
				}
			}
		}

		// start workers for this batch
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go worker()
		}

		// enqueue batch jobs (global indices)
		go func(s, e int) {
			for i := s; i < e; i++ {
				jobs <- i
			}
			close(jobs)
		}(start, end)

		// wait for workers to finish in goroutine
		go func() {
			wg.Wait()
			close(results)
			close(errorsCh)
		}()

		// collect batch results
		processed := 0
		for processed < batchSize {
			select {
			case r, ok := <-results:
				if !ok {
					// results closed; drain errors
					for err := range errorsCh {
						_ = err
					}
					processed = batchSize
					break
				}
				globalMu.Lock()
				stats.TotalPerStatus[r.StatusCode]++
				if _, exists := stats.ExampleMessage[r.StatusCode]; !exists {
					msg := r.Body
					if len(msg) > 200 {
						msg = msg[:200] + "..."
					}
					stats.ExampleMessage[r.StatusCode] = msg
				}
				globalMu.Unlock()
				processed++
			case err, ok := <-errorsCh:
				if !ok {
					processed = batchSize
					break
				}
				globalMu.Lock()
				stats.TotalPerStatus[0]++
				if _, exists := stats.ExampleMessage[0]; !exists {
					stats.ExampleMessage[0] = err.Error()
				}
				globalMu.Unlock()
				processed++
			case <-time.After(1 * time.Second):
				// allow progress even if channels quiet
			}
		}

		// batch done — close per-batch channels and proceed to next chunk
		// (wg already waited in goroutine, results/errors are drained)
	}

	// close log file if used
	if logging && csvFile != nil {
		csvWriter.Flush()
		_ = csvFile.Close()
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
