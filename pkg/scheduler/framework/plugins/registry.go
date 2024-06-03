/*
Copyright 2019 The Kubernetes Authors.

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

package plugins

import (
	"k8s.io/apiserver/pkg/util/feature"

	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultpreemption"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/dynamicresources"
	plfeature "k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/imagelocality"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/interpodaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodename"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeports"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/noderesources"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeunschedulable"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodevolumelimits"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/podtopologyspread"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/schedulinggates"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/tainttoleration"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumebinding"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumerestrictions"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumezone"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

// NewInTreeRegistry builds the registry with all the in-tree plugins.
// A scheduler that runs out of tree plugins can register additional plugins
// through the WithFrameworkOutOfTreeRegistry option.
// in-tree插件的注册表
func NewInTreeRegistry() runtime.Registry {
	fts := plfeature.Features{
		// feature.DefaultFeatureGate来启动或禁用某些功能
		EnableDynamicResourceAllocation:              feature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation),              // 动态资源分配
		EnableVolumeCapacityPriority:                 feature.DefaultFeatureGate.Enabled(features.VolumeCapacityPriority),                 // volume优先级调度
		EnableMinDomainsInPodTopologySpread:          feature.DefaultFeatureGate.Enabled(features.MinDomainsInPodTopologySpread),          // Pod 拓扑分布的最小域
		EnableNodeInclusionPolicyInPodTopologySpread: feature.DefaultFeatureGate.Enabled(features.NodeInclusionPolicyInPodTopologySpread), // Pod 拓扑分布中的节点包含策略
		EnableMatchLabelKeysInPodTopologySpread:      feature.DefaultFeatureGate.Enabled(features.MatchLabelKeysInPodTopologySpread),      // Pod 拓扑分布中匹配标签键
		EnablePodSchedulingReadiness:                 feature.DefaultFeatureGate.Enabled(features.PodSchedulingReadiness),
		EnablePodDisruptionConditions:                feature.DefaultFeatureGate.Enabled(features.PodDisruptionConditions),
		EnableInPlacePodVerticalScaling:              feature.DefaultFeatureGate.Enabled(features.InPlacePodVerticalScaling),
		EnableSidecarContainers:                      feature.DefaultFeatureGate.Enabled(features.SidecarContainers),
	}

	registry := runtime.Registry{
		dynamicresources.Name:                runtime.FactoryAdapter(fts, dynamicresources.New),                // 动态资源分配, 允许调度器在调度时考虑动态分配的资源
		imagelocality.Name:                   imagelocality.New,                                                // 镜像亲和性, 优化调度策略以利用本地镜像
		tainttoleration.Name:                 tainttoleration.New,                                              // 污点容忍度, 允许pod容忍节点上的污点
		nodename.Name:                        nodename.New,                                                     // 节点名称, 确保pod调度到指定的节点
		nodeports.Name:                       nodeports.New,                                                    // 节点端口, 允许pod使用节点端口
		nodeaffinity.Name:                    nodeaffinity.New,                                                 // 节点亲和性, 根据节点的label来调度pod
		podtopologyspread.Name:               runtime.FactoryAdapter(fts, podtopologyspread.New),               // Pod 拓扑分布，确保 Pod 在集群中分布均匀
		nodeunschedulable.Name:               nodeunschedulable.New,                                            // 节点不可调度, 防止pod调度到不可调度的节点
		noderesources.Name:                   runtime.FactoryAdapter(fts, noderesources.NewFit),                // 节点资源, 限制pod对节点的资源使用
		noderesources.BalancedAllocationName: runtime.FactoryAdapter(fts, noderesources.NewBalancedAllocation), // 节点资源平衡分配, 确保节点资源使用平衡
		volumebinding.Name:                   runtime.FactoryAdapter(fts, volumebinding.New),
		volumerestrictions.Name:              runtime.FactoryAdapter(fts, volumerestrictions.New),
		volumezone.Name:                      volumezone.New,
		nodevolumelimits.CSIName:             runtime.FactoryAdapter(fts, nodevolumelimits.NewCSI),
		nodevolumelimits.EBSName:             runtime.FactoryAdapter(fts, nodevolumelimits.NewEBS),
		nodevolumelimits.GCEPDName:           runtime.FactoryAdapter(fts, nodevolumelimits.NewGCEPD),
		nodevolumelimits.AzureDiskName:       runtime.FactoryAdapter(fts, nodevolumelimits.NewAzureDisk),
		nodevolumelimits.CinderName:          runtime.FactoryAdapter(fts, nodevolumelimits.NewCinder),
		interpodaffinity.Name:                interpodaffinity.New,
		queuesort.Name:                       queuesort.New,
		defaultbinder.Name:                   defaultbinder.New,
		defaultpreemption.Name:               runtime.FactoryAdapter(fts, defaultpreemption.New),
		schedulinggates.Name:                 runtime.FactoryAdapter(fts, schedulinggates.New),
	}

	return registry
}
