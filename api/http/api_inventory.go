// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/go-ozzo/ozzo-validation/v4"
	midentity "github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	u "github.com/mendersoftware/go-lib-micro/rest_utils"
	"github.com/pkg/errors"

	inventory "github.com/mendersoftware/inventory/inv"
	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
	"github.com/mendersoftware/inventory/utils"
	"github.com/mendersoftware/inventory/utils/identity"
)

const (
	uriDevices       = "/api/0.1.0/devices"
	uriDevice        = "/api/0.1.0/devices/:id"
	uriDeviceGroups  = "/api/0.1.0/devices/:id/group"
	uriDeviceGroup   = "/api/0.1.0/devices/:id/group/:name"
	uriAttributes    = "/api/0.1.0/attributes"
	uriGroups        = "/api/0.1.0/groups"
	uriGroupsDevices = "/api/0.1.0/groups/:name/devices"

	uriInternalTenants       = "/api/internal/v1/inventory/tenants"
	uriInternalDevices       = "/api/internal/v1/inventory/devices"
	urlInternalDevicesStatus = "/api/internal/v1/inventory/tenants/:tenant_id/devices/:status"
	urlInternalAttributes    = "/api/internal/v1/inventory/tenants/:tenant_id/device/:did/attribute/scope/:scope"
	apiUrlManagementV2       = "/api/management/v2/inventory"
	urlFiltersSearch         = apiUrlManagementV2 + "/filters/search"

	apiUrlInternalV2         = "/api/internal/v2/inventory"
	urlInternalFiltersSearch = apiUrlInternalV2 + "/tenants/:tenant_id/filters/search"

	hdrTotalCount = "X-Total-Count"
)

const (
	queryParamGroup          = "group"
	queryParamSort           = "sort"
	queryParamHasGroup       = "has_group"
	queryParamValueSeparator = ":"
	queryParamScopeSeparator = "/"
	sortOrderAsc             = "asc"
	sortOrderDesc            = "desc"
	sortAttributeNameIdx     = 0
	sortOrderIdx             = 1
)

// model of device's group name response at /devices/:id/group endpoint
type InventoryApiGroup struct {
	Group model.GroupName `json:"group"`
}

func (g InventoryApiGroup) Validate() error {
	return g.Group.Validate()
}

type inventoryHandlers struct {
	inventory inventory.InventoryApp
}

// return an ApiHandler for device admission app
func NewInventoryApiHandlers(i inventory.InventoryApp) ApiHandler {
	return &inventoryHandlers{
		inventory: i,
	}
}

func (i *inventoryHandlers) GetApp() (rest.App, error) {
	routes := []*rest.Route{
		rest.Get(uriDevices, i.GetDevicesHandler),
		rest.Get(uriDevice, i.GetDeviceHandler),
		rest.Delete(uriDevice, i.DeleteDeviceHandler),
		rest.Delete(uriDeviceGroup, i.DeleteDeviceGroupHandler),
		rest.Delete(uriGroupsDevices, i.ClearDevicesGroup),
		rest.Patch(uriAttributes, i.PatchDeviceAttributesHandler),
		rest.Patch(urlInternalAttributes, i.PatchDeviceAttributesInternalHandler),
		rest.Put(uriDeviceGroups, i.AddDeviceToGroupHandler),
		rest.Patch(uriGroupsDevices, i.AppendDevicesToGroup),
		rest.Get(uriDeviceGroups, i.GetDeviceGroupHandler),
		rest.Get(uriGroups, i.GetGroupsHandler),
		rest.Get(uriGroupsDevices, i.GetDevicesByGroup),

		rest.Post(uriInternalTenants, i.CreateTenantHandler),
		rest.Post(uriInternalDevices, i.AddDeviceHandler),
		rest.Post(urlInternalDevicesStatus, i.InternalDevicesStatusHandler),
		rest.Post(urlFiltersSearch, i.FiltersSearchHandler),

		rest.Post(urlInternalFiltersSearch, i.InternalFiltersSearchHandler),
	}

	routes = append(routes)

	app, err := rest.MakeRouter(
		// augment routes with OPTIONS handler
		AutogenOptionsRoutes(routes, AllowHeaderOptionsGenerator)...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create router")
	}

	return app, nil

}

// `sort` paramater value is an attribute name with optional direction (desc or asc)
// separated by colon (:)
//
// eg. `sort=attr_name1` or `sort=attr_name1:asc`
func parseSortParam(r *rest.Request) (*store.Sort, error) {
	sortStr, err := utils.ParseQueryParmStr(r, queryParamSort, false, nil)
	if err != nil {
		return nil, err
	}
	if sortStr == "" {
		return nil, nil
	}
	sortValArray := strings.Split(sortStr, queryParamValueSeparator)
	attrNameWithScope := strings.SplitN(sortValArray[sortAttributeNameIdx], queryParamScopeSeparator, 2)
	var scope, attrName string
	if len(attrNameWithScope) == 1 {
		scope = model.AttrScopeInventory
		attrName = attrNameWithScope[0]
	} else {
		scope = attrNameWithScope[0]
		attrName = attrNameWithScope[1]
	}
	sort := store.Sort{AttrName: attrName, AttrScope: scope}
	if len(sortValArray) == 2 {
		sortOrder := sortValArray[sortOrderIdx]
		if sortOrder != sortOrderAsc && sortOrder != sortOrderDesc {
			return nil, errors.New("invalid sort order")
		}
		sort.Ascending = sortOrder == sortOrderAsc
	}
	return &sort, nil
}

// Filter paramaters name are attributes name. Value can be prefixed
// with equality operator code (`eq` for =), separated from value by colon (:).
// Equality operator default value is `eq`
//
// eg. `attr_name1=value1` or `attr_name1=eq:value1`
func parseFilterParams(r *rest.Request) ([]store.Filter, error) {
	knownParams := []string{utils.PageName, utils.PerPageName, queryParamSort, queryParamHasGroup, queryParamGroup}
	filters := make([]store.Filter, 0)
	var filter store.Filter
	for name := range r.URL.Query() {
		if utils.ContainsString(name, knownParams) {
			continue
		}
		valueStr, err := utils.ParseQueryParmStr(r, name, false, nil)
		if err != nil {
			return nil, err
		}

		attrNameWithScope := strings.SplitN(name, queryParamScopeSeparator, 2)
		var scope, attrName string
		if len(attrNameWithScope) == 1 {
			scope = model.AttrScopeInventory
			attrName = attrNameWithScope[0]
		} else {
			scope = attrNameWithScope[0]
			attrName = attrNameWithScope[1]
		}
		filter = store.Filter{AttrName: attrName, AttrScope: scope}

		// make sure we parse ':'s in value, it's either:
		// not there
		// after a valid operator specifier
		// or/and inside the value itself(mac, etc), in which case leave it alone
		sepIdx := strings.Index(valueStr, ":")
		if sepIdx == -1 {
			filter.Value = valueStr
			filter.Operator = store.Eq
		} else {
			validOps := []string{"eq"}
			for _, o := range validOps {
				if valueStr[:sepIdx] == o {
					switch o {
					case "eq":
						filter.Operator = store.Eq
						filter.Value = valueStr[sepIdx+1:]
					}
					break
				}
			}

			if filter.Value == "" {
				filter.Value = valueStr
				filter.Operator = store.Eq
			}
		}

		floatValue, err := strconv.ParseFloat(filter.Value, 64)
		if err == nil {
			filter.ValueFloat = &floatValue
		}

		filters = append(filters, filter)
	}
	return filters, nil
}

func (i *inventoryHandlers) GetDevicesHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	page, perPage, err := utils.ParsePagination(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	hasGroup, err := utils.ParseQueryParmBool(r, queryParamHasGroup, false, nil)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	groupName, err := utils.ParseQueryParmStr(r, "group", false, nil)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	sort, err := parseSortParam(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	filters, err := parseFilterParams(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	ld := store.ListQuery{Skip: int((page - 1) * perPage),
		Limit:     int(perPage),
		Filters:   filters,
		Sort:      sort,
		HasGroup:  hasGroup,
		GroupName: groupName}

	devs, totalCount, err := i.inventory.ListDevices(ctx, ld)

	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	hasNext := totalCount > int(page*perPage)
	links := utils.MakePageLinkHdrs(r, page, perPage, hasNext)
	for _, l := range links {
		w.Header().Add("Link", l)
	}
	// the response writer will ensure the header name is in Kebab-Pascal-Case
	w.Header().Add("X-Total-Count", strconv.Itoa(totalCount))
	w.WriteJson(devs)
}

func (i *inventoryHandlers) GetDeviceHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	deviceID := r.PathParam("id")

	dev, err := i.inventory.GetDevice(ctx, model.DeviceID(deviceID))
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}
	if dev == nil {
		u.RestErrWithLog(w, r, l, store.ErrDevNotFound, http.StatusNotFound)
		return
	}

	w.WriteJson(dev)
}

func (i *inventoryHandlers) DeleteDeviceHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	deviceID := r.PathParam("id")

	err := i.inventory.DeleteDevice(ctx, model.DeviceID(deviceID))
	if err != nil && err != store.ErrDevNotFound {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (i *inventoryHandlers) AddDeviceHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	dev, err := parseDevice(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	err = dev.Attributes.Validate()
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	err = i.inventory.AddDevice(ctx, dev)
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.Header().Add("Location", "devices/"+dev.ID.String())
	w.WriteHeader(http.StatusCreated)
}

func (i *inventoryHandlers) PatchDeviceAttributesHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	//get device ID from JWT token
	idata, err := identity.ExtractIdentityFromHeaders(r.Header)
	if err != nil {
		u.RestErrWithLogMsg(w, r, l, err, http.StatusUnauthorized, "unauthorized")
		return
	}

	//extract attributes from body
	attrs, err := parseAttributes(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	//upsert the attributes
	err = i.inventory.UpsertAttributes(ctx, model.DeviceID(idata.Subject), attrs)
	cause := errors.Cause(err)
	switch cause {
	case store.ErrNoAttrName:
		u.RestErrWithLog(w, r, l, cause, http.StatusBadRequest)
		return
	}
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (i *inventoryHandlers) PatchDeviceAttributesInternalHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()
	tenantId := r.PathParam("tenant_id")
	ctx = getTenantContext(ctx, tenantId)

	l := log.FromContext(ctx)

	//extract attributes from body
	attrs, err := parseAttributes(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}
	for _,a := range attrs {
		a.Scope=r.PathParam("scope")
	}

	//upsert the attributes
	err = i.inventory.UpsertAttributes(ctx, model.DeviceID(r.PathParam("did")), attrs)
	cause := errors.Cause(err)
	switch cause {
	case store.ErrNoAttrName:
		u.RestErrWithLog(w, r, l, cause, http.StatusBadRequest)
		return
	}
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (i *inventoryHandlers) DeleteDeviceGroupHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	deviceID := r.PathParam("id")
	groupName := r.PathParam("name")

	err := i.inventory.UnsetDeviceGroup(ctx, model.DeviceID(deviceID), model.GroupName(groupName))
	if err != nil {
		cause := errors.Cause(err)
		if cause != nil {
			if cause.Error() == store.ErrDevNotFound.Error() {
				u.RestErrWithLog(w, r, l, err, http.StatusNotFound)
				return
			}
		}
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (i *inventoryHandlers) AddDeviceToGroupHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	devId := r.PathParam("id")

	var group InventoryApiGroup
	err := r.DecodeJsonPayload(&group)
	if err != nil {
		u.RestErrWithLog(
			w, r, l, errors.Wrap(err, "failed to decode device group data"),
			http.StatusBadRequest)
		return
	}

	if err = group.Validate(); err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	err = i.inventory.UpdateDeviceGroup(ctx, model.DeviceID(devId), model.GroupName(group.Group))
	if err != nil {
		if cause := errors.Cause(err); cause != nil && cause == store.ErrDevNotFound {
			u.RestErrWithLog(w, r, l, err, http.StatusNotFound)
			return
		}
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (i *inventoryHandlers) GetDevicesByGroup(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	group := r.PathParam("name")

	page, perPage, err := utils.ParsePagination(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	//get one extra device to see if there's a 'next' page
	ids, totalCount, err := i.inventory.ListDevicesByGroup(ctx, model.GroupName(group), int((page-1)*perPage), int(perPage))
	if err != nil {
		if err == store.ErrGroupNotFound {
			u.RestErrWithLog(w, r, l, err, http.StatusNotFound)
		} else {
			u.RestErrWithLogInternal(w, r, l, err)
		}
		return
	}

	hasNext := totalCount > int(page*perPage)

	links := utils.MakePageLinkHdrs(r, page, perPage, hasNext)
	for _, l := range links {
		w.Header().Add("Link", l)
	}
	// the response writer will ensure the header name is in Kebab-Pascal-Case
	w.Header().Add("X-Total-Count", strconv.Itoa(totalCount))
	w.WriteJson(ids)
}

func (i *inventoryHandlers) AppendDevicesToGroup(w rest.ResponseWriter, r *rest.Request) {
	var deviceIDs []model.DeviceID
	ctx := r.Context()
	l := log.FromContext(ctx)
	groupName := model.GroupName(r.PathParam("name"))
	if err := groupName.Validate(); err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	if err := r.DecodeJsonPayload(&deviceIDs); err != nil {
		u.RestErrWithLog(w, r, l,
			errors.Wrap(err, "invalid payload schema"),
			http.StatusBadRequest,
		)
		return
	} else if len(deviceIDs) == 0 {
		u.RestErrWithLog(w, r, l,
			errors.New("no device IDs present in payload"),
			http.StatusBadRequest,
		)
		return
	}
	updated, err := i.inventory.UpdateDevicesGroup(
		ctx, deviceIDs, groupName,
	)
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}
	w.WriteJson(updated)
}

func (i *inventoryHandlers) ClearDevicesGroup(w rest.ResponseWriter, r *rest.Request) {
	var deviceIDs []model.DeviceID
	ctx := r.Context()
	l := log.FromContext(ctx)

	groupName := model.GroupName(r.PathParam("name"))
	if err := groupName.Validate(); err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	if err := r.DecodeJsonPayload(&deviceIDs); err != nil {
		u.RestErrWithLog(w, r, l,
			errors.Wrap(err, "invalid payload schema"),
			http.StatusBadRequest,
		)
		return
	} else if len(deviceIDs) == 0 {
		u.RestErrWithLog(w, r, l,
			errors.New("no device IDs present in payload"),
			http.StatusBadRequest,
		)
		return
	}

	updated, err := i.inventory.UnsetDevicesGroup(ctx, deviceIDs, groupName)
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.WriteJson(updated)
}

func parseDevice(r *rest.Request) (*model.Device, error) {
	dev := model.Device{}

	//decode body
	err := r.DecodeJsonPayload(&dev)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	if err := dev.Validate(); err != nil {
		return nil, err
	}

	return &dev, nil
}

func parseAttributes(r *rest.Request) (model.DeviceAttributes, error) {
	var attrs model.DeviceAttributes

	err := r.DecodeJsonPayload(&attrs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	err = attrs.Validate()
	if err != nil {
		return nil, err
	}

	return attrs, nil
}

func (i *inventoryHandlers) GetGroupsHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	groups, err := i.inventory.ListGroups(ctx)
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	if groups == nil {
		groups = []model.GroupName{}
	}

	w.WriteJson(groups)
}

func (i *inventoryHandlers) GetDeviceGroupHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	deviceID := r.PathParam("id")

	group, err := i.inventory.GetDeviceGroup(ctx, model.DeviceID(deviceID))
	if err != nil {
		if err == store.ErrDevNotFound {
			u.RestErrWithLog(w, r, l, store.ErrDevNotFound, http.StatusNotFound)
		} else {
			u.RestErrWithLogInternal(w, r, l, err)
		}
		return
	}

	ret := map[string]*model.GroupName{"group": nil}

	if group != "" {
		ret["group"] = &group
	}

	w.WriteJson(ret)
}

type newTenantRequest struct {
	TenantID string `json:"tenant_id" valid:"required"`
}

func (t newTenantRequest) Validate() error {
	return validation.ValidateStruct(&t,
		validation.Field(&t.TenantID, validation.Required),
	)
}

func (i *inventoryHandlers) CreateTenantHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	var newTenant newTenantRequest

	if err := r.DecodeJsonPayload(&newTenant); err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	if err := newTenant.Validate(); err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	err := i.inventory.CreateTenant(ctx, model.NewTenant{
		ID: newTenant.TenantID,
	})
	if err != nil {
		u.RestErrWithLogInternal(w, r, l, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (i *inventoryHandlers) FiltersSearchHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	//extract attributes from body
	searchParams, err := parseSearchParams(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	// query database
	devs, totalCount, err := i.inventory.SearchDevices(ctx, *searchParams)
	if err != nil {
		if strings.Contains(err.Error(), "BadValue") {
			u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		} else {
			u.RestErrWithLogInternal(w, r, l, err)
		}
		return
	}

	// the response writer will ensure the header name is in Kebab-Pascal-Case
	w.Header().Add(hdrTotalCount, strconv.Itoa(totalCount))
	w.WriteJson(devs)
}

func (i *inventoryHandlers) InternalFiltersSearchHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	tenantId := r.PathParam("tenant_id")
	if tenantId == "" {
		u.RestErrWithLog(w, r, l, errors.New("tenant_id not provided"), http.StatusBadRequest)
		return
	}
	ctx = getTenantContext(ctx, tenantId)

	//extract attributes from body
	searchParams, err := parseSearchParams(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	// query database
	devs, totalCount, err := i.inventory.SearchDevices(ctx, *searchParams)
	if err != nil {
		if strings.Contains(err.Error(), "BadValue") {
			u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		} else {
			u.RestErrWithLogInternal(w, r, l, err)
		}
		return
	}

	// the response writer will ensure the header name is in Kebab-Pascal-Case
	w.Header().Add(hdrTotalCount, strconv.Itoa(totalCount))
	w.WriteJson(devs)
}

func getTenantContext(ctx context.Context, tenantId string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tenantId == "" {
		return ctx
	}
	id := &midentity.Identity{
		Tenant: tenantId,
	}

	ctx = midentity.WithContext(ctx, id)

	return ctx
}

func (i *inventoryHandlers) InternalDevicesStatusHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	status := r.PathParam("status")
	if status == "" {
		u.RestErrWithLog(w, r, l, errors.New("status not provided"), http.StatusBadRequest)
		return
	}

	tenantId := r.PathParam("tenant_id")

	ctx = getTenantContext(ctx, tenantId)

	var ids []string
	err := r.DecodeJsonPayload(&ids)
	if err != nil {
		u.RestErrWithLog(w, r, l, errors.Wrap(err, "cant parse device ids"), http.StatusBadRequest)
		return
	}

	for _, id := range ids {
		attrs := model.DeviceAttributes{
			{
				Name:        "status",
				Description: nil,
				Value:       status,
				Scope:       model.AttrScopeIdentity,
			},
		}
		//upsert the attributes
		err = i.inventory.UpsertAttributes(ctx, model.DeviceID(id), attrs)
		if err != nil {
			u.RestErrWithLogInternal(w, r, l, err)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func parseSearchParams(r *rest.Request) (*model.SearchParams, error) {
	var searchParams model.SearchParams

	if err := r.DecodeJsonPayload(&searchParams); err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	if searchParams.Page < 1 {
		searchParams.Page = utils.PageDefault
	}
	if searchParams.PerPage < 1 {
		searchParams.PerPage = utils.PerPageDefault
	}

	if err := searchParams.Validate(); err != nil {
		return nil, err
	}

	return &searchParams, nil
}
