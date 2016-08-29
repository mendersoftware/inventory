// Copyright 2016 Mender Software AS
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

package utils

import (
	"github.com/ant0ine/go-json-rest/rest"
	"net/url"
	"strings"
)

// build URL using request 'r' and template, replace path params with
// elements from 'params' using lexical match as in strings.Replace()
func BuildURL(r *rest.Request, template string, params map[string]string) *url.URL {
	url := r.BaseUrl()

	path := template
	for k, v := range params {
		path = strings.Replace(path, k, v, -1)
	}
	url.Path = path

	return url
}
