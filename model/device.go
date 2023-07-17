// Copyright 2023 Northern.tech AS
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

package model

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	AttrScopeInventory = "inventory"
	AttrScopeIdentity  = "identity"
	AttrScopeSystem    = "system"
	AttrScopeTags      = "tags"
	AttrScopeMonitor   = "monitor"

	AttrNameID             = "id"
	AttrNameGroup          = "group"
	AttrNameUpdated        = "updated_ts"
	AttrNameCreated        = "created_ts"
	AttrNameTagsEtag       = "tags_etag"
	AttrNameNumberOfAlerts = "alert_count"
	AttrNameAlerts         = "alerts"
)

const (
	runeDollar = '\uFF04'
	runeDot    = '\uFF0E'
)

var validGroupNameRegex = regexp.MustCompile("^[A-Za-z0-9_-]*$")

type DeviceID string

var NilDeviceID DeviceID //TODO: how to make it NilDeviceID:=DeviceID(primitive.NilObjectID)

type GroupName string

type DeviceGroups struct {
	Groups []string `json:"groups" bson:"-"`
}

type DeviceAttribute struct {
	Name        string      `json:"name" bson:",omitempty"`
	Description *string     `json:"description,omitempty" bson:",omitempty"`
	Value       interface{} `json:"value" bson:",omitempty"`
	Scope       string      `json:"scope" bson:",omitempty"`
	Timestamp   *time.Time  `json:"timestamp,omitempty" bson:",omitempty"`
}

func (da DeviceAttributes) GetByName(name string) *DeviceAttribute {
	for _, attribute := range da {
		if attribute.Name == name {
			rc := attribute
			return &rc
		}
	}
	return nil
}

func (da DeviceAttributes) ToMap(excludeNames ...string) map[string]DeviceAttribute {
	rc := make(map[string]DeviceAttribute, len(da))
	for _, v := range da {
		exclude := false
		for _, excluded := range excludeNames {
			if v.Name == excluded {
				exclude = true
				break
			}
		}
		if exclude {
			continue
		}
		rc[v.Scope+"_"+v.Name] = v
	}
	return rc
}

func (da DeviceAttributes) Equal(existingAttributesArray DeviceAttributes) bool {
	// cannot be equal if they are of different size (the cheapest check first)
	if len(da)-2 != len(existingAttributesArray) { // -2 comes from updated and created timestamps
		return false
	}

	// cannot be equal if any of the values differ (the most common case)
	attributes := da.ToMap(AttrNameUpdated, AttrNameCreated)
	existingAttributes := existingAttributesArray.ToMap(AttrNameUpdated, AttrNameCreated)
	if len(attributes) != len(existingAttributes) {
		return false
	}
	for k, v := range attributes {
		if e, ok := existingAttributes[k]; ok {
			if !v.Equal(e) {
				return false
			}
		} else {
			// cannot be equal if keys differ (the last possibility)
			return false
		}
	}

	return true
}

func (da DeviceAttribute) Validate() error {
	return validation.ValidateStruct(&da,
		validation.Field(&da.Name, validation.Required, validation.Length(1, 1024)),
		validation.Field(&da.Scope, validation.Required, validation.Length(1, 1024)),
		validation.Field(&da.Value, validation.By(validateDeviceAttrVal)),
		validation.Field(&da.Timestamp, validation.Date(time.RFC3339)),
	)
}

func allowedType(v reflect.Value) bool {
	if v.Type().Kind() == reflect.Int {
		return true
	}
	if v.Type().Kind() == reflect.String {
		return true
	}
	if v.Type().Kind() == reflect.Float64 {
		return true
	}
	return false
}

func reflectValuesEqual(rVal1 reflect.Value, rVal2 reflect.Value) bool {
	if rVal1.Len() != rVal2.Len() {
		return false
	}
	for i := 0; i < rVal1.Len(); i++ {
		if rVal1.Index(i).Kind() != rVal2.Index(i).Kind() {
			return false
		}
		if !allowedType(rVal1.Index(i)) || !allowedType(rVal2.Index(i)) {
			if !reflect.DeepEqual(rVal1.Index(i), rVal2.Index(i)) {
				return false
			}
		} else {
			if rVal2.Index(i).Kind() == reflect.Int {
				value1 := rVal1.Index(i).Interface().(int)
				value2 := rVal2.Index(i).Interface().(int)
				if value1 != value2 {
					return false
				}
			}
			if rVal2.Index(i).Kind() == reflect.String {
				value1 := rVal1.Index(i).Interface().(string)
				value2 := rVal2.Index(i).Interface().(string)
				if value1 != value2 {
					return false
				}
			}
			if rVal2.Index(i).Kind() == reflect.Float64 {
				floatComparePrecision := "%.8f"
				floatValue1 := rVal1.Index(i).Interface().(float64)
				floatValue2 := rVal2.Index(i).Interface().(float64)
				value1 := fmt.Sprintf(floatComparePrecision, floatValue1)
				value2 := fmt.Sprintf(floatComparePrecision, floatValue2)
				if value1 != value2 {
					return false
				}
			}
		}
	}
	return true
}

func (da DeviceAttribute) Equal(e DeviceAttribute) bool {
	if (da.Value == nil || e.Value == nil) && !(da.Value == nil && e.Value == nil) {
		return false
	}
	rVal1 := reflect.ValueOf(da.Value)
	rVal2 := reflect.ValueOf(e.Value)
	if rVal1.Kind() != rVal2.Kind() {
		if rVal1.Kind() == reflect.Slice && rVal2.Kind() == reflect.Array ||
			rVal1.Kind() == reflect.Array && rVal2.Kind() == reflect.Slice {
		} else {
			return false
		}
	}
	if rVal1.Kind() == reflect.Slice || rVal1.Kind() == reflect.Array {
		return reflectValuesEqual(rVal1, rVal2)
	} else {
		if da.Value != e.Value {
			return false
		}
		if da.Scope == e.Scope {
			if da.Name == e.Name {
				if da.Description == nil && e.Description == nil {
					// at this point Value, Scope, and Name are equal, and both Descriptions are nil
					return true
				}
				if da.Description == nil || e.Description == nil {
					// if either is nil while the other is not they are different
					return false
				}
				if *da.Description == *e.Description {
					// both not nil, we can compare and if equal the attributes are equal
					return true
				}
			}
		}
	}
	return false
}

func validateDeviceAttrVal(i interface{}) error {
	if i == nil {
		return errors.New("supported types are string, float64, and arrays thereof")
	}
	rType := reflect.TypeOf(i)
	if rType.Kind() == reflect.Interface {
		rType = rType.Elem()
	}

	switch rType.Kind() {
	case reflect.Float64, reflect.String:
		return nil
	case reflect.Slice:
		elemKind := rType.Elem().Kind()
		if elemKind == reflect.Float64 || elemKind == reflect.String {
			return nil
		} else if elemKind == reflect.Interface {
			return validateDeviceAttrValArray(i)
		}
	}
	return errors.New("supported types are string, float64, and arrays thereof")
}

func validateDeviceAttrValArray(arr interface{}) error {
	rVal := reflect.ValueOf(arr)
	rLen := rVal.Len()
	if rLen == 0 {
		return nil
	}
	elem := rVal.Index(0)
	kind := elem.Kind()
	if elem.Kind() == reflect.Interface {
		elem = elem.Elem()
		kind = elem.Kind()
	}
	if kind != reflect.String && kind != reflect.Float64 {
		return errors.New(
			"array values must be either string or float64, not: " +
				kind.String())
	}
	for i := 1; i < rLen; i++ {
		elem = rVal.Index(i)
		elemKind := elem.Kind()
		if elemKind == reflect.Interface {
			elemKind = elem.Elem().Kind()
		}
		if elemKind != kind {
			return errors.New(
				"array values must be of consistent type: " +
					"string or float64",
			)
		}
	}
	return nil
}

// Device wrapper
type Device struct {
	//system-generated device ID
	ID DeviceID `json:"id" bson:"_id,omitempty"`

	//a map of attributes names and their values.
	Attributes DeviceAttributes `json:"attributes,omitempty" bson:"attributes,omitempty"`

	//device's group name
	Group GroupName `json:"-" bson:"group,omitempty"`

	CreatedTs time.Time `json:"-" bson:"created_ts,omitempty"`
	//Timestamp of the last attribute update.
	UpdatedTs time.Time `json:"updated_ts" bson:"updated_ts,omitempty"`

	//device object revision
	Revision uint `json:"-" bson:"revision,omitempty"`

	//tags attributes ETag
	TagsEtag string `json:"-" bson:"tags_etag,omitempty"`

	//text attribute for the full-text search
	Text string `json:"-" bson:"text,omitempty"`
}

// internalDevice is only used internally to avoid recursive type-loops for
// member functions.
type internalDevice Device

func (d *Device) UnmarshalBSON(b []byte) error {
	if err := bson.Unmarshal(b, (*internalDevice)(d)); err != nil {
		return err
	}
	for _, attr := range d.Attributes {
		if attr.Scope == AttrScopeSystem {
			switch attr.Name {
			case AttrNameGroup:
				group := attr.Value.(string)
				d.Group = GroupName(group)
			case AttrNameUpdated:
				dateTime := attr.Value.(primitive.DateTime)
				d.UpdatedTs = dateTime.Time()
			case AttrNameCreated:
				dateTime := attr.Value.(primitive.DateTime)
				d.CreatedTs = dateTime.Time()
			}
		}
	}
	return nil
}

func (d Device) MarshalBSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if d.Group != "" {
		d.Attributes = append(d.Attributes, DeviceAttribute{
			Scope: AttrScopeSystem,
			Name:  AttrNameGroup,
			Value: d.Group,
		})
	}
	return bson.Marshal(internalDevice(d))
}

func (d Device) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.ID, validation.Required, validation.Length(1, 1024)),
		validation.Field(&d.Attributes),
		validation.Field(&d.TagsEtag, validation.Length(0, 1024)),
	)
}

func (did DeviceID) String() string {
	return string(did)
}

func (gn GroupName) String() string {
	return string(gn)
}

func (gn GroupName) Validate() error {
	if len(gn) > 1024 {
		return errors.New(
			"Group name can at most have 1024 characters",
		)
	} else if len(gn) == 0 {
		return errors.New(
			"Group name cannot be blank",
		)
	} else if !validGroupNameRegex.MatchString(string(gn)) {
		return errors.New(
			"Group name can only contain: upper/lowercase " +
				"alphanum, -(dash), _(underscore)",
		)
	}
	return nil
}

// wrapper for device attributes names and values
type DeviceAttributes []DeviceAttribute

func (d *DeviceAttributes) UnmarshalJSON(b []byte) error {
	err := json.Unmarshal(b, (*[]DeviceAttribute)(d))
	if err != nil {
		return err
	}
	for i := range *d {
		if (*d)[i].Scope == "" {
			(*d)[i].Scope = AttrScopeInventory
		}
	}

	return nil
}

// MarshalJSON ensures that an empty array is returned if DeviceAttributes is
// empty.
func (d DeviceAttributes) MarshalJSON() ([]byte, error) {
	if d == nil {
		return json.Marshal([]DeviceAttribute{})
	}
	return json.Marshal([]DeviceAttribute(d))
}

func (d DeviceAttributes) Validate() error {
	for _, a := range d {
		if err := a.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func GetDeviceAttributeNameReplacer() *strings.Replacer {
	return strings.NewReplacer(".", string(runeDot), "$", string(runeDollar))
}

// UnmarshalBSONValue correctly unmarshals DeviceAttributes from Device
// documents stored in the DB.
func (d *DeviceAttributes) UnmarshalBSONValue(t bsontype.Type, b []byte) error {
	raw := bson.Raw(b)
	elems, err := raw.Elements()
	if err != nil {
		return err
	}
	*d = make(DeviceAttributes, len(elems))
	for i, elem := range elems {
		err = elem.Value().Unmarshal(&(*d)[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// MarshalBSONValue marshals the DeviceAttributes to a mongo-compatible
// document. That is, each attribute is given a unique field consisting of
// "<scope>-<name>".
func (d DeviceAttributes) MarshalBSONValue() (bsontype.Type, []byte, error) {
	attrs := make(bson.D, len(d))
	replacer := GetDeviceAttributeNameReplacer()
	for i := range d {
		attr := DeviceAttribute{
			Name:        d[i].Name,
			Description: d[i].Description,
			Value:       d[i].Value,
			Scope:       d[i].Scope,
			Timestamp:   d[i].Timestamp,
		}
		attrs[i].Key = attr.Scope + "-" + replacer.Replace(d[i].Name)
		attrs[i].Value = &attr
	}
	return bson.MarshalValue(attrs)
}

type DeviceUpdate struct {
	Id       DeviceID `json:"id"`
	Revision uint     `json:"revision"`
}
