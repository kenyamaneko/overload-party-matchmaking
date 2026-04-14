package secretmanager

import (
	"context"
	"fmt"

	sm "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// Client は Google Cloud Secret Manager への薄いラッパです。
// Workload Identity で bind された Service Account の認証情報を使います。
type Client struct {
	projectID string
	client    *sm.Client
}

// NewClient は Secret Manager クライアントを生成します。
func NewClient(ctx context.Context, projectID string) (*Client, error) {
	if projectID == "" {
		return nil, fmt.Errorf("secretmanager: projectID is empty")
	}
	c, err := sm.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("secretmanager: new client: %w", err)
	}
	return &Client{projectID: projectID, client: c}, nil
}

// AccessLatest は指定 secret の latest version を取得します。
func (c *Client) AccessLatest(ctx context.Context, secretID string) (string, error) {
	if secretID == "" {
		return "", fmt.Errorf("secretmanager: secretID is empty")
	}
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", c.projectID, secretID)
	resp, err := c.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: name})
	if err != nil {
		return "", fmt.Errorf("secretmanager: access %s: %w", secretID, err)
	}
	return string(resp.Payload.Data), nil
}

// Close は Secret Manager クライアントを閉じます。
func (c *Client) Close() error {
	return c.client.Close()
}
