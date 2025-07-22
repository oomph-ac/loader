package api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var (
	clientCert *x509.Certificate
	apiClient  *http.Client

	bodyMaxSize = 1024 * 1024 // 1 MB
	bodyPool    = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, 1024*1024))
		},
	}
)

func init() {
	tlsCert, err := tls.LoadX509KeyPair("oomph-api-client.crt", "oomph-api-client.key")
	if err != nil {
		fmt.Printf("Failed to load TLS certificate: %v\n", err)
		return
	}
	apiClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				MinVersion:   tls.VersionTLS12,
				MaxVersion:   tls.VersionTLS13,
			},
		},
		Timeout: time.Minute,
	}
}

func CallEndpoint[T any](
	endpoint string,
	requestData any,
	onSuccess func(T),
	onFailure func(message string),
	onError func(err error),
) {
	bodyDat := bodyPool.Get().(*bytes.Buffer)
	bodyDat.Reset()
	defer func() {
		if bodyDat.Cap() <= bodyMaxSize {
			bodyPool.Put(bodyDat)
		}
	}()

	if requestData != nil {
		if err := json.NewEncoder(bodyDat).Encode(requestData); err != nil {
			onError(fmt.Errorf("failed to encode request data: %w", err))
			return
		}
	}

	req, err := http.NewRequest("POST", endpoint, bodyDat)
	if err != nil {
		onError(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := apiClient.Do(req)
	if err != nil {
		onError(fmt.Errorf("failed to send request: %w", err))
		return
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	if res.StatusCode == http.StatusOK {
		var data T
		if err := decoder.Decode(&data); err != nil {
			onError(err)
			return
		}
		onSuccess(data)
	} else {
		var errResp ErrorResponse
		if err := decoder.Decode(&errResp); err != nil {
			onError(fmt.Errorf("server responded with status code %d with no message", res.StatusCode))
			return
		}
		onFailure(errResp.Message)
	}
}
