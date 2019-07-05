// Copyright 2019 Northern.tech AS
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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/go-ozzo/ozzo-validation"
	id "github.com/mendersoftware/go-lib-micro/identity"
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
	uriDevices       = "/api/management/v2/inventory/devices"
	uriDevicesV1     = "/api/0.1.0/devices"
	uriDevice        = "/api/0.1.0/devices/:id"
	uriDeviceGroups  = "/api/0.1.0/devices/:id/group"
	uriDeviceGroup   = "/api/0.1.0/devices/:id/group/:name"
	uriAttributes    = "/api/0.1.0/attributes"
	uriGroups        = "/api/0.1.0/groups"
	uriGroupsDevices = "/api/0.1.0/groups/:name/devices"

	uriInternalTenants = "/api/internal/v1/inventory/tenants"
	uriInternalDevices = "/api/internal/v1/inventory/devices"

	uriInternalAttributes = "/api/internal/v2/inventory/devices/:id"
)

const (
	queryParamGroup          = "group"
	queryParamSort           = "sort"
	queryParamHasGroup       = "has_group"
	queryParamValueSeparator = ":"
	queryParamScopeSeparator = ":"
	sortOrderAsc             = "asc"
	sortOrderDesc            = "desc"
	sortScopeIdx             = 0
	sortAttributeNameIdx     = 1
	sortOrderIdx             = 2
	sortAttributeNameIdxV1   = 0
	sortOrderIdxV1           = 1
)

// model of device's group name response at /devices/:id/group endpoint
type InventoryApiGroup struct {
	Group string `json:"group"`
}

func (g InventoryApiGroup) Validate() error {
	return validation.ValidateStruct(&g,
		validation.Field(&g.Group, validation.Required),
	)
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
		rest.Get(uriDevicesV1, i.GetDevicesV1Handler),
		rest.Get(uriDevice, i.GetDeviceV1Handler),
		rest.Delete(uriDevice, i.DeleteDeviceHandler),
		rest.Delete(uriDeviceGroup, i.DeleteDeviceGroupHandler),
		rest.Patch(uriAttributes, i.PatchDeviceAttributesHandler),
		rest.Put(uriDeviceGroups, i.AddDeviceToGroupHandler),
		rest.Get(uriDeviceGroups, i.GetDeviceGroupHandler),
		rest.Get(uriGroups, i.GetGroupsHandler),
		rest.Get(uriGroupsDevices, i.GetDevicesByGroup),

		rest.Post(uriInternalTenants, i.CreateTenantHandler),
		rest.Post(uriInternalDevices, i.AddDeviceHandler),
		rest.Patch(uriInternalAttributes, i.InternalPatchDeviceAttributesHandler),
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

// parseSortParam parses the v2 version of Sort, e.g.
// `sort=identity:attr_name1`
// `sort=identity:attr_name1:asc`
func parseSortParam(r *rest.Request) (*store.Sort, error) {
	sortStr, err := utils.ParseQueryParmStr(r, queryParamSort, false, nil)
	if err != nil {
		return nil, err
	}
	if sortStr == "" {
		return nil, nil
	}

	sortValArray := strings.Split(sortStr, queryParamValueSeparator)
	if len(sortValArray) < 2 {
		return nil, fmt.Errorf("invalid sort '%s': must include at minimum scope and name (e.g. 'identity:mac')", sortStr)
	}

	sort := store.Sort{}

	scope := sortValArray[sortScopeIdx]
	if scope != model.AttrScopeIdentity {
		return nil, errors.New("supported attribute scopes: [ identity ]")
	}
	name := sortValArray[sortAttributeNameIdx]

	sort.AttrName = fmt.Sprintf("%s-%s", scope, name)

	// 3 elems - scope:name:dir
	if len(sortValArray) == 3 {
		sortOrder := sortValArray[sortOrderIdx]
		if sortOrder != sortOrderAsc && sortOrder != sortOrderDesc {
			return nil, errors.New("invalid sort order")
		}
		sort.Ascending = sortOrder == sortOrderAsc
	}

	return &sort, nil
}

// parseFilterParams parses the v2 version of filter descriptor, e.g.:
// `scope:attr_name1=value1`
// `scope:attr_name1=eq:value1`
func parseFilterParams(r *rest.Request) ([]store.Filter, error) {
	knownParams := []string{utils.PageName, utils.PerPageName, queryParamSort, queryParamHasGroup, queryParamGroup}
	filters := make([]store.Filter, 0)
	var filter store.Filter
	for name := range r.URL.Query() {
		if utils.ContainsString(name, knownParams) {
			continue
		}
		// name is 'scope:attr_name'
		nameArr := strings.Split(name, queryParamValueSeparator)
		if len(nameArr) != 2 {
			return nil, fmt.Errorf("invalid filter '%s': must include scope and name (e.g. 'identity:mac')", name)
		}
		scope := nameArr[0]
		if scope != model.AttrScopeIdentity {
			return nil, errors.New("supported attribute scopes: [ identity ]")
		}

		attr := nameArr[1]

		valueStr, err := utils.ParseQueryParmStr(r, name, false, nil)
		if err != nil {
			return nil, err
		}

		filter = store.Filter{
			AttrName: fmt.Sprintf("%s-%s", scope, attr),
		}

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

func (i *inventoryHandlers) GetDevicesV1Handler(w rest.ResponseWriter, r *rest.Request) {
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

	sort, err := parseSortParamV1(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	filters, err := parseFilterParamsV1(r)
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
	limitDevicesAttrs(devs, model.AttrScopeInventory)

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

func (i *inventoryHandlers) GetDeviceV1Handler(w rest.ResponseWriter, r *rest.Request) {
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
	limitDeviceAttrs(dev, model.AttrScopeInventory)

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
	attrs, err := parseDeviceAttributes(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	//upsert the attributes
	err = i.inventory.UpsertAttributes(ctx, model.DeviceID(idata.Subject), attrs)
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

	if !regexp.MustCompile("^[A-Za-z0-9_-]*$").MatchString(group.Group) {
		u.RestErrWithLog(w, r, l, errors.New("Group name can only contain: upper/lowercase alphanum, -(dash), _(underscore)"), http.StatusBadRequest)
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

func parseDeviceAttributes(r *rest.Request) (model.DeviceAttributes, error) {
	var attrs model.DeviceAttributes

	err := r.DecodeJsonPayload(&attrs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	for i := range attrs {
		a := attrs[i]
		a.Scope = model.AttrScopeInventory
		attrs[i] = a
		if err = a.Validate(); err != nil {
			return nil, err
		}
	}

	return attrs, nil
}

func parseAttributes(r *rest.Request) (model.DeviceAttributes, error) {
	var attrs model.DeviceAttributes

	err := r.DecodeJsonPayload(&attrs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	for _, a := range attrs {
		if err = a.Validate(); err != nil {
			return nil, err
		}
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

func (i *inventoryHandlers) InternalPatchDeviceAttributesHandler(w rest.ResponseWriter, r *rest.Request) {
	ctx := r.Context()

	l := log.FromContext(ctx)

	deviceId := r.PathParam("id")

	tenant, err := utils.ParseQueryParmStr(r, "tenant_id", false, nil)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	if tenant != "" {
		ctx = id.WithContext(ctx, &id.Identity{
			Tenant: tenant,
		})
	}

	timestamp, err := parseTimestampHeader(r.Header.Get("X-MEN-Msg-Timestamp"))
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	serviceName := r.Header.Get("X-MEN-Source")
	if len(serviceName) == 0 {
		u.RestErrWithLog(w, r, l, errors.New("Required X-MEN-Source header is missing"), http.StatusBadRequest)
		return
	}

	source := model.AttributeSource{
		Name:      serviceName,
		Timestamp: timestamp,
	}

	//extract attributes from body
	attrs, err := parseAttributes(r)
	if err != nil {
		u.RestErrWithLog(w, r, l, err, http.StatusBadRequest)
		return
	}

	//upsert the attributes
	err = i.inventory.UpsertAttributesWithSource(ctx, model.DeviceID(deviceId), attrs, source)
	if err != nil {
		if err == store.ErrAttrPatchOutdated {
			u.RestErrWithLog(w, r, l, err, http.StatusPreconditionFailed)
		} else {
			u.RestErrWithLogInternal(w, r, l, err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func parseTimestampHeader(header string) (uint64, error) {

	if header == "" {
		return 0, errors.New("Required X-MEN-Msg-Timestamp header missing")
	}

	uintVal, err := strconv.ParseUint(header, 10, 64)
	if err != nil {
		return 0, errors.New("X-MEN-Msg-Timestamp header invalid (UNIX timestamp with miliseconds expected).")
	}

	return uintVal, nil
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

	// transform devices to DTOs
	out := make([]*DeviceDto, len(devs), len(devs))
	for i, d := range devs {
		out[i] = NewDeviceDto(&d)
	}

	w.WriteJson(out)
}

// method to

// `sort` paramater value is an attribute name with optional direction (desc or asc)
// separated by colon (:)
//
// eg. `sort=attr_name1` or `sort=attr_name1:asd`
// V1 version of parseSortParam method is using inventory attributes only
func parseSortParamV1(r *rest.Request) (*store.Sort, error) {
	sortStr, err := utils.ParseQueryParmStr(r, queryParamSort, false, nil)
	if err != nil {
		return nil, err
	}
	if sortStr == "" {
		return nil, nil
	}
	sortValArray := strings.Split(sortStr, queryParamValueSeparator)
	name := fmt.Sprintf("%s-%s", model.AttrScopeInventory, sortValArray[sortAttributeNameIdxV1])
	sort := store.Sort{AttrName: name}
	if len(sortValArray) == 2 {
		sortOrder := sortValArray[sortOrderIdxV1]
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
// V1 version of parseSortParam method is using inventory attributes only
func parseFilterParamsV1(r *rest.Request) ([]store.Filter, error) {
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

		name = fmt.Sprintf("%s-%s", model.AttrScopeInventory, name)
		filter = store.Filter{AttrName: name}

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

// remove attributes with scope different than "inventory"
func limitDeviceAttrs(dev *model.Device, scope string) {
	for k, a := range dev.Attributes {
		if a.Scope != model.AttrScopeInventory {
			delete(dev.Attributes, k)
		}
	}
	return
}

func limitDevicesAttrs(devs []model.Device, scope string) {
	for _, dev := range devs {
		limitDeviceAttrs(&dev, scope)
	}
	return
}
