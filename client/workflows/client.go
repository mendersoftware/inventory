// Copyright 2021 Northern.tech AS
//
//    All Rights Reserved

package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/requestid"
	"github.com/mendersoftware/go-lib-micro/rest_utils"
	"github.com/pkg/errors"
)

const (
	ReindexURI = "/api/v1/workflow/reindex_reporting"
	HealthURI  = "/api/v1/health"
)

const (
	defaultTimeout = time.Duration(5) * time.Second
)

// Client is the workflows client
//go:generate ../../utils/mockgen.sh
type Client interface {
	CheckHealth(ctx context.Context) error
	StartReindex(c context.Context, device string) error
}

type ClientOptions struct {
	Client *http.Client
}

// NewClient returns a new workflows client
func NewClient(url string, opts ...ClientOptions) Client {
	// Initialize default options
	var clientOpts = ClientOptions{
		Client: &http.Client{},
	}
	// Merge options
	for _, opt := range opts {
		if opt.Client != nil {
			clientOpts.Client = opt.Client
		}
	}

	return &client{
		url:    strings.TrimSuffix(url, "/"),
		client: *clientOpts.Client,
	}
}

type client struct {
	url    string
	client http.Client
}

func (c *client) StartReindex(ctx context.Context, device string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	id := identity.FromContext(ctx)
	if id == nil || id.Tenant == "" {
		return errors.New("workflows: Context lacking tenant identity")
	}
	wflow := ReindexWorkflow{
		RequestID: requestid.FromContext(ctx),
		TenantID:  id.Tenant,
		DeviceID:  device,
		Service:   ServiceInventory,
	}
	payload, _ := json.Marshal(wflow)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		c.url+ReindexURI,
		bytes.NewReader(payload),
	)
	if err != nil {
		return errors.Wrap(err, "workflows: error preparing HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	rsp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "workflows: failed to submit auditlog")
	}

	if rsp.StatusCode < 300 {
		return nil
	}

	if rsp.StatusCode == http.StatusNotFound {
		return errors.New(`workflows: workflow "reindex_reporting" not defined`)
	}

	return errors.Errorf(
		"workflows: unexpected HTTP status from workflows service: %s",
		rsp.Status,
	)
}

func (c *client) CheckHealth(ctx context.Context) error {
	var (
		apiErr rest_utils.ApiError
	)

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	req, _ := http.NewRequestWithContext(
		ctx, "GET",
		joinURL(c.url, HealthURI), nil,
	)

	rsp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if rsp.StatusCode >= http.StatusOK && rsp.StatusCode < 300 {
		return nil
	}
	decoder := json.NewDecoder(rsp.Body)
	err = decoder.Decode(&apiErr)
	if err != nil {
		return errors.Errorf("health check HTTP error: %s", rsp.Status)
	}
	return &apiErr
}

func joinURL(base, url string) string {
	if strings.HasPrefix(url, "/") {
		url = url[1:]
	}
	if !strings.HasSuffix(base, "/") {
		base = base + "/"
	}
	return base + url

}
