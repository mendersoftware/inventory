// Copyright 2021 Northern.tech AS
//
//    All Rights Reserved
package workflows

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/requestid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func mockServerReindex(t *testing.T, tenant, device, reqid string, code int) (*httptest.Server, error) {
	h := func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		defer r.Body.Close()

		request := ReindexWorkflow{}

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&request)
		assert.NoError(t, err)

		assert.Equal(t, reqid, request.RequestID)
		assert.Equal(t, tenant, request.TenantID)
		assert.Equal(t, device, request.DeviceID)
		assert.Equal(t, ServiceInventory, request.Service)

		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}

	srv := httptest.NewServer(http.HandlerFunc(h))
	return srv, nil
}

func TestReindex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		tenant string
		device string
		reqid  string

		code int

		err error
	}{
		{
			name:   "ok",
			tenant: "tenant1",
			device: "device2",
			reqid:  "reqid1",

			code: http.StatusOK,
		},
		{
			name:   "500",
			tenant: "tenant2",
			device: "device3",
			reqid:  "reqid2",

			code: http.StatusNotFound,
			err:  errors.New(`workflows: workflow "reindex_reporting" not defined`),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv, err := mockServerReindex(t, tc.tenant, tc.device, tc.reqid, tc.code)
			assert.NoError(t, err)

			defer srv.Close()

			ctx := context.Background()
			ctx = requestid.WithContext(ctx, tc.reqid)
			ctx = identity.WithContext(ctx,
				&identity.Identity{
					Tenant: tc.tenant,
				})

			client := NewClient(srv.URL)

			err = client.StartReindex(ctx, tc.device)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
