//Copyright 2020 Northern.tech AS
//
//All Rights Reserved

package model

import (
	"github.com/go-ozzo/ozzo-validation"
)

var validSelectors = []interface{}{
	"$eq",
	"$gt",
	"$gte",
	"$in",
	"$lt",
	"$lte",
	"$ne",
	"$nin",
	"$exists"}

var validSortOrders = []interface{}{"asc", "desc"}

type SearchParams struct {
	Page    int               `json:"page"`
	PerPage int               `json:"per_page"`
	Filters []FilterPredicate `json:"filters"`
	Sort    []SortCriteria    `json:"sort"`
}

type FilterPredicate struct {
	Scope     string      `json:"scope"`
	Attribute string      `json:"attribute"`
	Type      string      `json:"type"`
	Value     interface{} `json:"value"`
}

type SortCriteria struct {
	Scope     string `json:"scope"`
	Attribute string `json:"attribute"`
	Order     string `json:"order"`
}

func (sp SearchParams) Validate() error {
	for _, f := range sp.Filters {
		err := validation.ValidateStruct(&f,
			validation.Field(&f.Scope, validation.Required),
			validation.Field(&f.Attribute, validation.Required),
			validation.Field(&f.Type, validation.Required, validation.In(validSelectors...)),
			validation.Field(&f.Value, validation.Required))
		if err != nil {
			return err
		}
	}

	for _, s := range sp.Sort {
		err := validation.ValidateStruct(&s,
			validation.Field(&s.Scope, validation.Required),
			validation.Field(&s.Attribute, validation.Required),
			validation.Field(&s.Order, validation.Required, validation.In(validSortOrders...)))
		if err != nil {
			return err
		}
	}
	return nil
}
