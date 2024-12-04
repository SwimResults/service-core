package client

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func Post(url string, path string, data interface{}, header *http.Header) (*http.Response, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequest("POST", url+path, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	if header != nil {
		r.Header = *header
	}
	r.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func Get(url string, path string, params map[string]string, header *http.Header) (*http.Response, error) {
	r, err := http.NewRequest("GET", url+path, nil)
	if err != nil {
		return nil, err
	}

	if header != nil {
		r.Header = *header
	}

	q := r.URL.Query()
	for key, value := range params {
		q.Add(key, value)
	}
	r.URL.RawQuery = q.Encode()
	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	return res, nil
}
