// Package apimatchmakingclient は consumer から wire 詳細 (REST path / HTTP status code) を
// 隠蔽するため、生成 client を sentinel error 変換でラップする。
package apimatchmakingclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// Sentinel errors. wire 契約 (OpenAPI spec) で定義された status code の意味を sentinel として export する。
var (
	// ErrNotFound は status 404。
	ErrNotFound = errors.New("apimatchmakingclient: not found")

	// ErrUnauthorized は status 401。
	ErrUnauthorized = errors.New("apimatchmakingclient: unauthorized")

	// ErrForbidden は status 403。
	ErrForbidden = errors.New("apimatchmakingclient: forbidden")

	// ErrBadRequest は status 400。
	ErrBadRequest = errors.New("apimatchmakingclient: bad request")

	// ErrServiceUnavailable は status 503。matchmaking サービスが受付停止中の場合に返る。
	ErrServiceUnavailable = errors.New("apimatchmakingclient: service unavailable")

	// ErrInternalServer は status 5xx (503 を除く)。
	ErrInternalServer = errors.New("apimatchmakingclient: internal server error")
)

// Client は overload-party-matchmaking REST API の sentinel-error converted client。
type Client struct {
	api *apimatchmaking.ClientWithResponses
}

// Option は Client 構築時の設定。
type Option func(*config)

type config struct {
	httpClient apimatchmaking.HttpRequestDoer
	editors    []apimatchmaking.RequestEditorFn
}

// WithHTTPClient は基底 HTTP doer を差し替える。デフォルトは http.DefaultClient。
func WithHTTPClient(doer apimatchmaking.HttpRequestDoer) Option {
	return func(c *config) { c.httpClient = doer }
}

// WithRequestEditorFn は全リクエストに適用する RequestEditor を追加する。
func WithRequestEditorFn(fn apimatchmaking.RequestEditorFn) Option {
	return func(c *config) { c.editors = append(c.editors, fn) }
}

// New は baseURL に接続する Client を生成する。
func New(baseURL string, opts ...Option) (*Client, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	apiOpts := make([]apimatchmaking.ClientOption, 0, 1+len(cfg.editors))
	if cfg.httpClient != nil {
		apiOpts = append(apiOpts, apimatchmaking.WithHTTPClient(cfg.httpClient))
	}
	for _, ed := range cfg.editors {
		apiOpts = append(apiOpts, apimatchmaking.WithRequestEditorFn(ed))
	}

	api, err := apimatchmaking.NewClientWithResponses(baseURL, apiOpts...)
	if err != nil {
		return nil, fmt.Errorf("apimatchmakingclient: new: %w", err)
	}
	return &Client{api: api}, nil
}

// GetHealth はサービス死活監視用エンドポイントを叩く。
func (c *Client) GetHealth(ctx context.Context) (*apimatchmaking.HealthResponse, error) {
	resp, err := c.api.GetHealthWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("apimatchmakingclient: GetHealth: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetHealth", resp.StatusCode())
}

// GetQueueSize は現在のマッチメイキング待機人数を返す。
func (c *Client) GetQueueSize(ctx context.Context) (*apimatchmaking.QueueSizeResponse, error) {
	resp, err := c.api.GetQueueSizeWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("apimatchmakingclient: GetQueueSize: %w", err)
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, statusError("GetQueueSize", resp.StatusCode())
}

// EnqueuePlayer はプレイヤーをマッチメイキングキューに登録する。spec は 202 Accepted を返す。
func (c *Client) EnqueuePlayer(ctx context.Context, req apimatchmaking.EnqueueRequest) error {
	resp, err := c.api.EnqueuePlayerWithResponse(ctx, apimatchmaking.EnqueuePlayerJSONRequestBody(req))
	if err != nil {
		return fmt.Errorf("apimatchmakingclient: EnqueuePlayer: %w", err)
	}
	if resp.StatusCode() == http.StatusAccepted {
		return nil
	}
	return statusError("EnqueuePlayer", resp.StatusCode())
}

// CancelPlayer はプレイヤーをキューから取り除く。
func (c *Client) CancelPlayer(ctx context.Context) error {
	resp, err := c.api.CancelPlayerWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("apimatchmakingclient: CancelPlayer: %w", err)
	}
	if resp.StatusCode() == http.StatusOK {
		return nil
	}
	return statusError("CancelPlayer", resp.StatusCode())
}

// statusError は HTTP status code を sentinel error (errors.Is 分岐可能) に変換する。
func statusError(op string, code int) error {
	var sentinel error
	switch {
	case code == http.StatusUnauthorized:
		sentinel = ErrUnauthorized
	case code == http.StatusForbidden:
		sentinel = ErrForbidden
	case code == http.StatusNotFound:
		sentinel = ErrNotFound
	case code == http.StatusBadRequest:
		sentinel = ErrBadRequest
	case code == http.StatusServiceUnavailable:
		sentinel = ErrServiceUnavailable
	case code >= http.StatusInternalServerError:
		sentinel = ErrInternalServer
	default:
		return fmt.Errorf("apimatchmakingclient: %s: unexpected status %d", op, code)
	}
	return fmt.Errorf("apimatchmakingclient: %s: %w", op, sentinel)
}
