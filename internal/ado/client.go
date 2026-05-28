package ado

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiVersion = "7.1"

type Client struct {
	org     string
	pat     string
	httpCli *http.Client
	baseURL string // defaults to "https://dev.azure.com"
}

func NewClient(org, pat string) *Client {
	return &Client{
		org:     org,
		pat:     pat,
		httpCli: &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://dev.azure.com",
	}
}

type WIQLResult struct {
	WorkItems []WIQLWorkItem `json:"workItems"`
}

type WIQLWorkItem struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

type WorkItem struct {
	ID        int                    `json:"id"`
	Rev       int                    `json:"rev"`
	Fields    map[string]interface{} `json:"fields"`
	Relations []Relation             `json:"relations"`
}

type Relation struct {
	Rel        string                 `json:"rel"`
	URL        string                 `json:"url"`
	Attributes map[string]interface{} `json:"attributes"`
}

type BatchResult struct {
	Value []WorkItem `json:"value"`
	Count int        `json:"count"`
}

func (c *Client) RunWIQL(project, team, query string, top int) (*WIQLResult, error) {
	reqURL := fmt.Sprintf("%s/%s/%s/%s/_apis/wit/wiql?$top=%d&api-version=%s",
		c.baseURL, url.PathEscape(c.org), url.PathEscape(project), url.PathEscape(team), top, apiVersion)

	body := fmt.Sprintf(`{"query": %s}`, jsonString(query))

	req, err := http.NewRequest("POST", reqURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("WIQL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("WIQL failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result WIQLResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode WIQL response: %w", err)
	}
	return &result, nil
}

func (c *Client) GetWorkItems(project string, ids []int) ([]WorkItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var all []WorkItem
	for i := 0; i < len(ids); i += 200 {
		end := i + 200
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		items, err := c.fetchWorkItemBatch(project, batch)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}

	return all, nil
}

func (c *Client) fetchWorkItemBatch(project string, batch []int) ([]WorkItem, error) {
	idStrs := make([]string, len(batch))
	for j, id := range batch {
		idStrs[j] = fmt.Sprintf("%d", id)
	}

	reqURL := fmt.Sprintf("%s/%s/%s/_apis/wit/workitems?ids=%s&$expand=relations&api-version=%s",
		c.baseURL, url.PathEscape(c.org), url.PathEscape(project), strings.Join(idStrs, ","), apiVersion)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get work items: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("get work items failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result BatchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode work items: %w", err)
	}
	return result.Value, nil
}

func (c *Client) setAuth(req *http.Request) {
	encoded := base64.StdEncoding.EncodeToString([]byte(":" + c.pat))
	req.Header.Set("Authorization", "Basic "+encoded)
}

type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func (c *Client) UpdateWorkItem(project string, id int, ops []PatchOperation) (*WorkItem, error) {
	reqURL := fmt.Sprintf("%s/%s/%s/_apis/wit/workitems/%d?api-version=%s",
		c.baseURL, url.PathEscape(c.org), url.PathEscape(project), id, apiVersion)

	body, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("marshal patch ops: %w", err)
	}

	req, err := http.NewRequest("PATCH", reqURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json-patch+json")
	c.setAuth(req)

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update work item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("update work item %d failed (HTTP %d): %s", id, resp.StatusCode, string(respBody))
	}

	var result WorkItem
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode update response: %w", err)
	}
	return &result, nil
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
