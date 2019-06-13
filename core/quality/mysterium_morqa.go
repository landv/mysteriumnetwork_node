/*
 * Copyright (C) 2018 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package quality

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/cihub/seelog"
	"github.com/golang/protobuf/proto"
	"github.com/mysteriumnetwork/metrics"
)

const (
	mysteriumMorqaLogPrefix = "[Mysterium.morqa] "
	mysteriumMorqaAgentName = "goclient-v0.1"
)

// HttpClient sends actual HTTP requests
type HttpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// MysteriumMORQA HTTP client for Mysterium QualityOracle - MORQA
type MysteriumMORQA struct {
	http    HttpClient
	baseURL string
}

// NewMorqaClient creates Mysterium Morqa client with a real communication
func NewMorqaClient(baseURL string, timeout time.Duration) *MysteriumMORQA {
	return &MysteriumMORQA{
		http:    &http.Client{Timeout: timeout},
		baseURL: baseURL,
	}
}

// ProposalsMetrics returns a list of proposals connection metrics
func (m *MysteriumMORQA) ProposalsMetrics() []json.RawMessage {
	request, err := m.newRequestJSON(http.MethodGet, "proposals/quality", nil)
	if err != nil {
		log.Warn(mysteriumMorqaLogPrefix, "Failed to create proposals metrics request: ", err)
		return nil
	}

	response, err := m.http.Do(request)
	if err != nil {
		log.Warn(mysteriumMorqaLogPrefix, err)
		return nil
	}
	defer response.Body.Close()

	var metricsResponse ServiceMetricsResponse
	if err = parseResponseJSON(response, &metricsResponse); err != nil {
		log.Warn(mysteriumMorqaLogPrefix, err)
		return nil
	}

	return metricsResponse.Connects
}

// SentEvent metric event to oracle
func (m *MysteriumMORQA) SendMetric(event *metrics.Event) error {
	request, err := m.newRequestBinary(http.MethodPost, "metrics", event)
	if err != nil {
		log.Warn(mysteriumMorqaLogPrefix, err)
		return err
	}

	response, err := m.http.Do(request)
	if err != nil {
		log.Warn(mysteriumMorqaLogPrefix, err)
		return err
	}
	defer response.Body.Close()

	if err = parseResponseError(response); err != nil {
		log.Warn(mysteriumMorqaLogPrefix, err)
		return err
	}

	return nil
}

func (m *MysteriumMORQA) newRequest(method, path string, body []byte) (*http.Request, error) {
	url := m.baseURL
	if len(path) > 0 {
		url = fmt.Sprintf("%v/%v", url, path)
	}

	request, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	request.Header.Set("User-Agent", mysteriumMorqaAgentName)
	request.Header.Set("Accept", "application/json")
	return request, err
}

func (m *MysteriumMORQA) newRequestJSON(method, path string, payload interface{}) (*http.Request, error) {
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := m.newRequest(method, path, payloadBody)
	req.Header.Set("Accept", "application/json")
	return req, err
}

func (m *MysteriumMORQA) newRequestBinary(method, path string, payload proto.Message) (*http.Request, error) {
	payloadBody, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}

	request, err := m.newRequest(method, path, payloadBody)
	request.Header.Set("Content-Type", "application/octet-stream")
	return request, nil
}

func parseResponseJSON(response *http.Response, dto interface{}) error {
	responseJSON, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(responseJSON, dto)
}

type errorDTO struct {
	Message string `json:"message"`
}

// Sometimes we can get json message with single "message" field which represents error - try to get that
func parseResponseError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	var parsedBody errorDTO
	var message string
	err := parseResponseJSON(response, &parsedBody)
	if err != nil {
		message = err.Error()
	} else {
		message = parsedBody.Message
	}
	return fmt.Errorf("server response invalid: %s (%s). Possible error: %s", response.Status, response.Request.URL, message)
}
