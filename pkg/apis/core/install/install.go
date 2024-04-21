/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package install installs the v1 monolithic api, making it available as an
// option to all of the API encoding/decoding machinery.
package install

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/core"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
)

func init() {
	// 注册core资源组的内部版本和外部版本
	Install(legacyscheme.Scheme)
}

// Install registers the API group and adds types to a scheme
func Install(scheme *runtime.Scheme) {
	// 注册core资源组内部版本的资源（例：apps/_internal/）
	utilruntime.Must(core.AddToScheme(scheme))
	// 注册core资源组外部版本的资源（例：apps/{v1,v1beta1,v1beta2}）
	utilruntime.Must(v1.AddToScheme(scheme))
	// 注册资源组的版本顺序（如果有多个版本，排在对前面的为资源首选版本）
	utilruntime.Must(scheme.SetVersionPriority(v1.SchemeGroupVersion))
}
