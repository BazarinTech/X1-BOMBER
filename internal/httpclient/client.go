package httpclient

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

type Response struct {
	Status     string
	StatusCode int
	Body       string
}

func SendRequest(method, url, body string, headers map[string]string) (*Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, _ := io.ReadAll(res.Body)
	return &Response{
		Status:     res.Status,
		StatusCode: res.StatusCode,
		Body:       string(data),
	}, nil
}
