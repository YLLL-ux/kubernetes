/*
Copyright 2015 The Kubernetes Authors.

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

package workqueue

import (
	"sync"
	"time"

	"k8s.io/utils/clock"
)

// Interface FIFO队列
type Interface interface {
	Add(item interface{})                   // 添加一个元素
	Len() int                               // 元素个数
	Get() (item interface{}, shutdown bool) // 获取一个元素，第二个返回值和channel类似，标记队列是否关闭了
	Done(item interface{})                  // 标记一个元素已经处理完
	ShutDown()                              // 关闭队列
	ShutDownWithDrain()                     // 关闭队列，但是等待队列中元素处理完
	ShuttingDown() bool                     // 标记当前channel是否正在关闭
}

// QueueConfig specifies optional configurations to customize an Interface.
type QueueConfig struct {
	// Name for the queue. If unnamed, the metrics will not be registered.
	Name string

	// MetricsProvider optionally allows specifying a metrics provider to use for the queue
	// instead of the global provider.
	MetricsProvider MetricsProvider

	// Clock ability to inject real or fake clock for testing purposes.
	Clock clock.WithTicker
}

// New constructs a new work queue (see the package comment).
func New() *Type {
	return NewWithConfig(QueueConfig{
		Name: "",
	})
}

// NewWithConfig constructs a new workqueue with ability to
// customize different properties.
func NewWithConfig(config QueueConfig) *Type {
	return newQueueWithConfig(config, defaultUnfinishedWorkUpdatePeriod)
}

// NewNamed creates a new named queue.
// Deprecated: Use NewWithConfig instead.
func NewNamed(name string) *Type {
	return NewWithConfig(QueueConfig{
		Name: name,
	})
}

// newQueueWithConfig constructs a new named workqueue
// with the ability to customize different properties for testing purposes
func newQueueWithConfig(config QueueConfig, updatePeriod time.Duration) *Type {
	var metricsFactory *queueMetricsFactory
	if config.MetricsProvider != nil {
		metricsFactory = &queueMetricsFactory{
			metricsProvider: config.MetricsProvider,
		}
	} else {
		metricsFactory = &globalMetricsFactory
	}

	if config.Clock == nil {
		config.Clock = clock.RealClock{}
	}

	return newQueue(
		config.Clock,
		metricsFactory.newQueueMetrics(config.Name, config.Clock),
		updatePeriod,
	)
}

func newQueue(c clock.WithTicker, metrics queueMetrics, updatePeriod time.Duration) *Type {
	t := &Type{
		clock:                      c,
		dirty:                      set{},
		processing:                 set{},
		cond:                       sync.NewCond(&sync.Mutex{}),
		metrics:                    metrics,
		unfinishedWorkUpdatePeriod: updatePeriod,
	}

	// Don't start the goroutine for a type of noMetrics so we don't consume
	// resources unnecessarily
	if _, ok := metrics.(noMetrics); !ok {
		go t.updateUnfinishedWorkLoop()
	}

	return t
}

const defaultUnfinishedWorkUpdatePeriod = 500 * time.Millisecond

// Type is a work queue (see the package comment).
type Type struct {
	// queue defines the order in which we will work on items. Every
	// element of queue should be in the dirty set and not in the
	// processing set.
	// 存储元素的处理顺序，里面所有元素在dirty集合中应该都有，而不能出现在processing集群中
	queue []t

	// dirty defines all of the items that need to be processed.
	// 标记所有需要被处理的元素
	dirty set

	// Things that are currently being processed are in the processing set.
	// These things may be simultaneously in the dirty set. When we finish
	// processing something and remove it from this set, we'll check if
	// it's in the dirty set, and if so, add it to the queue.
	// 当前正在被处理的元素，当处理完后，需要检查该元素是否在dirty集合中，如果在则添加到queue队列中
	processing set // 存放的是当前正在处理的元素，也就是说这个元素来自queue出队的元素，同时这个元素也会被从dirty中删除

	cond *sync.Cond

	shuttingDown bool
	drain        bool

	metrics queueMetrics

	unfinishedWorkUpdatePeriod time.Duration
	clock                      clock.WithTicker
}

type empty struct{}
type t interface{}
type set map[t]empty

func (s set) has(item t) bool {
	_, exists := s[item]
	return exists
}

func (s set) insert(item t) {
	s[item] = empty{}
}

func (s set) delete(item t) {
	delete(s, item)
}

func (s set) len() int {
	return len(s)
}

// Add marks item as needing processing.
func (q *Type) Add(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if q.shuttingDown { // 如果queue正在被关闭，则直接返回
		return
	}
	if q.dirty.has(item) { // 如果dirty set中已经存在该元素，则直接返回
		return
	}

	q.metrics.add(item)

	q.dirty.insert(item)        // 添加到dirty set中
	if q.processing.has(item) { // 如果正在被处理，则返回
		return
	}

	// 如果没有正在被处理，则添加到queue队列中
	q.queue = append(q.queue, item)
	q.cond.Signal() // 通知getter有新的元素
}

// Len returns the current queue length, for informational purposes only. You
// shouldn't e.g. gate a call to Add() or Get() on Len() being a particular
// value, that can't be synchronized properly.
func (q *Type) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return len(q.queue)
}

// Get blocks until it can return an item to be processed. If shutdown = true,
// the caller should end their goroutine. You must call Done with item when you
// have finished processing it.
func (q *Type) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	// 如果queue为空，并且没有正在关闭，则等待下一个元素的到来
	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	// 这时如果queue的长度还是0，则说明shuttingDown为true，所以直接返回
	if len(q.queue) == 0 {
		// We must be shutting down.
		return nil, true
	}

	item = q.queue[0] // 获取队列queue的第一个元素
	// The underlying array still exists and reference this object, so the object will not be garbage collected.
	q.queue[0] = nil      // 这里置为nil是为了让底层数组不在引用该对象，从而可以进行GC，防止内存泄露
	q.queue = q.queue[1:] // 更新queue

	q.metrics.get(item)

	q.processing.insert(item) // 将刚才获取的第一个元素放到processing集合中
	q.dirty.delete(item)      // 在dirty集合中删除该元素

	return item, false
}

// Done marks item as done processing, and if it has been marked as dirty again
// while it was being processed, it will be re-added to the queue for
// re-processing.
func (q *Type) Done(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.metrics.done(item)

	q.processing.delete(item) // 在processing集合中删除该元素
	if q.dirty.has(item) {    // 如果dirty中还有，则说明还需要再次处理，放到queue中
		q.queue = append(q.queue, item)
		q.cond.Signal() // 通知getter有新的元素
	} else if q.processing.len() == 0 {
		q.cond.Signal()
	}
}

// ShutDown will cause q to ignore all new items added to it and
// immediately instruct the worker goroutines to exit.
func (q *Type) ShutDown() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.drain = false
	q.shuttingDown = true
	q.cond.Broadcast()
}

// ShutDownWithDrain will cause q to ignore all new items added to it. As soon
// as the worker goroutines have "drained", i.e: finished processing and called
// Done on all existing items in the queue; they will be instructed to exit and
// ShutDownWithDrain will return. Hence: a strict requirement for using this is;
// your workers must ensure that Done is called on all items in the queue once
// the shut down has been initiated, if that is not the case: this will block
// indefinitely. It is, however, safe to call ShutDown after having called
// ShutDownWithDrain, as to force the queue shut down to terminate immediately
// without waiting for the drainage.
func (q *Type) ShutDownWithDrain() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.drain = true
	q.shuttingDown = true
	q.cond.Broadcast()

	for q.processing.len() != 0 && q.drain {
		q.cond.Wait()
	}
}

func (q *Type) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	return q.shuttingDown
}

func (q *Type) updateUnfinishedWorkLoop() {
	t := q.clock.NewTicker(q.unfinishedWorkUpdatePeriod)
	defer t.Stop()
	for range t.C() {
		if !func() bool {
			q.cond.L.Lock()
			defer q.cond.L.Unlock()
			if !q.shuttingDown {
				q.metrics.updateUnfinishedWork()
				return true
			}
			return false

		}() {
			return
		}
	}
}
