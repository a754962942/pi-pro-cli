package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Config struct {
	ServerURL  string
	AuthToken  string
	HTTPClient HTTPClient
}

type Client struct {
	config Config
}

func New(config Config) *Client {
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Client{config: config}
}

func (c *Client) DoJSON(ctx context.Context, method string, path string, request any, response any) error {
	var body io.Reader
	if request != nil {
		data, err := json.Marshal(request)
		if err != nil {
			return apperror.AppError{Code: errdefs.CodeUsage, Message: "failed to encode request JSON", Kind: apperror.KindUsage}
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url(path), body)
	if err != nil {
		return apperror.AppError{Code: errdefs.CodeUsage, Message: err.Error(), Kind: apperror.KindUsage}
	}
	req.Header.Set("Accept", "application/json")
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthToken)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return apperror.AppError{Code: errdefs.CodeNetworkRequestFailed, Message: "server request failed", Kind: apperror.KindNetwork, Details: map[string]any{
			"error": err.Error(),
		}}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return c.serverError(resp)
	}

	if response == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return apperror.AppError{Code: errdefs.CodeServerResponseInvalid, Message: "server response was not valid JSON", Kind: apperror.KindNetwork, Details: map[string]any{
			"error": err.Error(),
		}}
	}
	return nil
}

func (c *Client) url(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(c.config.ServerURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) serverError(resp *http.Response) error {
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)

	details := map[string]any{
		"statusCode": float64(resp.StatusCode),
	}
	if body.Error.Code != "" {
		details["serverCode"] = body.Error.Code
	}
	if body.Error.Message != "" {
		details["serverMessage"] = body.Error.Message
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return apperror.AppError{
			Code:    errdefs.CodeAuthInvalid,
			Message: "auth token was rejected by the server",
			Kind:    apperror.KindAuth,
			Details: details,
		}
	}

	message := "server request failed"
	if body.Error.Message != "" {
		message = fmt.Sprintf("server request failed: %s", body.Error.Message)
	}
	return apperror.AppError{
		Code:    errdefs.CodeServerRequestFailed,
		Message: message,
		Kind:    apperror.KindNetwork,
		Details: details,
	}
}

func IsRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}
