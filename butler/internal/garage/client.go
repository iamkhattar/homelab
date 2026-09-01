package garage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: 15 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}}
}

type Node struct {
	ID   string         `json:"id"`
	IsUp bool           `json:"isUp"`
	Role map[string]any `json:"role"`
}

type ClusterStatus struct {
	LayoutVersion int64  `json:"layoutVersion"`
	Nodes         []Node `json:"nodes"`
}

type Bucket struct {
	ID string `json:"id"`
}
type AccessKey struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

func (c *Client) ClusterStatus(ctx context.Context) (ClusterStatus, error) {
	var out ClusterStatus
	return out, c.do(ctx, http.MethodGet, "/v2/GetClusterStatus", nil, &out)
}

func (c *Client) AssignNode(ctx context.Context, id, zone string, capacity int64) error {
	body := map[string]any{"roles": []map[string]any{{"id": id, "zone": zone, "capacity": capacity, "tags": []string{"single-node", "titan"}}}}
	return c.do(ctx, http.MethodPost, "/v2/UpdateClusterLayout", body, nil)
}

func (c *Client) ApplyLayout(ctx context.Context, version int64) error {
	return c.do(ctx, http.MethodPost, "/v2/ApplyClusterLayout", map[string]any{"version": version}, nil)
}

func (c *Client) Bucket(ctx context.Context, alias string) (*Bucket, error) {
	var out Bucket
	err := c.do(ctx, http.MethodGet, "/v2/GetBucketInfo?globalAlias="+url.QueryEscape(alias), nil, &out)
	if IsNotFound(err) {
		return nil, nil
	}
	return &out, err
}

func (c *Client) CreateBucket(ctx context.Context, alias string) (*Bucket, error) {
	var out Bucket
	err := c.do(ctx, http.MethodPost, "/v2/CreateBucket", map[string]any{"globalAlias": alias}, &out)
	return &out, err
}

func (c *Client) CreateKey(ctx context.Context, name string) (*AccessKey, error) {
	var out AccessKey
	err := c.do(ctx, http.MethodPost, "/v2/CreateKey", map[string]any{"name": name, "neverExpires": true}, &out)
	return &out, err
}

func (c *Client) DeleteKey(ctx context.Context, accessKeyID string) error {
	return c.do(ctx, http.MethodPost, "/v2/DeleteKey", map[string]any{"accessKeyId": accessKeyID}, nil)
}

func (c *Client) AllowBucketKey(ctx context.Context, bucketID, keyID string, read, write, owner bool) error {
	body := map[string]any{"bucketId": bucketID, "accessKeyId": keyID, "permissions": map[string]bool{"read": read, "write": write, "owner": owner}}
	return c.do(ctx, http.MethodPost, "/v2/AllowBucketKey", body, nil)
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("garage API returned %d: %s", e.Status, e.Body)
}
func IsNotFound(err error) bool {
	e, ok := err.(*HTTPError)
	return ok && e.Status == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling garage API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}
