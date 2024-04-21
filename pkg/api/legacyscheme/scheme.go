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

package legacyscheme

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// 1.初始化scheme资源注册表
var (
	// Scheme is the default instance of runtime.Scheme to which types in the Kubernetes API are already registered.
	// NOTE: If you are copying this file to start a new api group, STOP! Copy the
	// extensions group instead. This Scheme is special and should appear ONLY in
	// the api group, unless you really know what you're doing.
	// TODO(lavalamp): make the above error impossible.
	// 使用全局资源注册表legacyscheme.Scheme
	Scheme = runtime.NewScheme()

	// Codecs provides access to encoding and decoding for the scheme
	// 作用: 允许你将 Kubernetes 对象转换为可以在网络上传输的格式，或者从这些格式中重建 Kubernetes 对象。
	// 举例: 当你需要从一个 JSON 文件中读取一个 Pod 对象并将其发送到 Kubernetes API 服务器时，你会使用 Codecs 中的 JSON 解码器来解析该文件并创建一个 Pod 实例。同样地，如果你需要将一个 Kubernetes Service 对象转换为 YAML 格式以便在配置文件中使用，你会使用 Codecs 中的 YAML 编码器。
	Codecs = serializer.NewCodecFactory(Scheme)

	// ParameterCodec handles versioning of objects that are converted to query parameters.
	// 作用: 用于 API 请求中的查询参数版本控制和转换。这在需要将复杂的查询参数传递给 API 时非常有用，例如列表请求时的字段选择、标签选择或复杂过滤。
	// 举例: 假设你想要列出所有带有特定标签的 Pods。在 Kubernetes API 中，你可以通过添加查询参数来实现这一点。ParameterCodec 能够将标签选择器对象转换为查询字符串的一部分，例如 pods?labelSelector=env%3Dproduction。在服务器端，ParameterCodec 将被用来解析这个查询字符串，重建标签选择器，并据此返回匹配的 Pods 列表。
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)
