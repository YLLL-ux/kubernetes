/*
Copyright 2016 The Kubernetes Authors.

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
	"container/heap"
	"sync"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/clock"
)

// DelayingInterface is an Interface that can Add an item at a later time. This makes it easier to
// requeue items after failures without ending up in a hot-loop.
type DelayingInterface interface {
	Interface
	// AddAfter adds an item to the workqueue after the indicated duration has passed
	AddAfter(item interface{}, duration time.Duration)
}

// DelayingQueueConfig specifies optional configurations to customize a DelayingInterface.
type DelayingQueueConfig struct {
	// Name for the queue. If unnamed, the metrics will not be registered.
	Name string

	// MetricsProvider optionally allows specifying a metrics provider to use for the queue
	// instead of the global provider.
	MetricsProvider MetricsProvider

	// Clock optionally allows injecting a real or fake clock for testing purposes.
	Clock clock.WithTicker

	// Queue optionally allows injecting custom queue Interface instead of the default one.
	Queue Interface
}

// NewDelayingQueue constructs a new workqueue with delayed queuing ability.
// NewDelayingQueue does not emit metrics. For use with a MetricsProvider, please use
// NewDelayingQueueWithConfig instead and specify a name.
func NewDelayingQueue() DelayingInterface {
	return NewDelayingQueueWithConfig(DelayingQueueConfig{})
}

// NewDelayingQueueWithConfig constructs a new workqueue with options to
// customize different properties.
func NewDelayingQueueWithConfig(config DelayingQueueConfig) DelayingInterface {
	if config.Clock == nil {
		config.Clock = clock.RealClock{}
	}

	if config.Queue == nil {
		config.Queue = NewWithConfig(QueueConfig{
			Name:            config.Name,
			MetricsProvider: config.MetricsProvider,
			Clock:           config.Clock,
		})
	}

	return newDelayingQueue(config.Clock, config.Queue, config.Name, config.MetricsProvider)
}

// NewDelayingQueueWithCustomQueue constructs a new workqueue with ability to
// inject custom queue Interface instead of the default one
// Deprecated: Use NewDelayingQueueWithConfig instead.
func NewDelayingQueueWithCustomQueue(q Interface, name string) DelayingInterface {
	return NewDelayingQueueWithConfig(DelayingQueueConfig{
		Name:  name,
		Queue: q,
	})
}

// NewNamedDelayingQueue constructs a new named workqueue with delayed queuing ability.
// Deprecated: Use NewDelayingQueueWithConfig instead.
func NewNamedDelayingQueue(name string) DelayingInterface {
	return NewDelayingQueueWithConfig(DelayingQueueConfig{Name: name})
}

// NewDelayingQueueWithCustomClock constructs a new named workqueue
// with ability to inject real or fake clock for testing purposes.
// Deprecated: Use NewDelayingQueueWithConfig instead.
func NewDelayingQueueWithCustomClock(clock clock.WithTicker, name string) DelayingInterface {
	return NewDelayingQueueWithConfig(DelayingQueueConfig{
		Name:  name,
		Clock: clock,
	})
}

func newDelayingQueue(clock clock.WithTicker, q Interface, name string, provider MetricsProvider) *delayingType {
	ret := &delayingType{
		Interface:       q,
		clock:           clock,
		heartbeat:       clock.NewTicker(maxWait),
		stopCh:          make(chan struct{}),
		waitingForAddCh: make(chan *waitFor, 1000),
		metrics:         newRetryMetrics(name, provider),
	}

	go ret.waitingLoop() // 延时队列实现的核心逻辑
	return ret
}

// delayingType wraps an Interface and provides delayed re-enquing
type delayingType struct {
	Interface // 内嵌普通队列Queue

	// clock tracks time for delayed firing
	clock clock.Clock // 计时器

	// stopCh lets us signal a shutdown to the waiting loop
	stopCh chan struct{}
	// stopOnce guarantees we only signal shutdown a single time
	stopOnce sync.Once // 用来确保ShutDown只执行一次

	// heartbeat ensures we wait no more than maxWait before firing
	heartbeat clock.Ticker // 默认10s的心跳，后面用在一个大循环里，避免没有新元素时一直阻塞

	// waitingForAddCh is a buffered channel that feeds waitingForAdd
	waitingForAddCh chan *waitFor // 传递waitFor的channel，默认大小为1000

	// metrics counts the number of retries
	metrics retryMetrics
}

// waitFor holds the data to add and the time it should be added
type waitFor struct {
	data    t         // 准备添加到队列中的数据
	readyAt time.Time // 应该被加入队列的时间
	// index in the priority queue (heap)
	index int // 在heap中的索引
}

// waitForPriorityQueue implements a priority queue for waitFor items.
//
// waitForPriorityQueue implements heap.Interface. The item occurring next in
// time (i.e., the item with the smallest readyAt) is at the root (index 0).
// Peek returns this minimum item at index 0. Pop returns the minimum item after
// it has been removed from the queue and placed at index Len()-1 by
// container/heap. Push adds an item at index Len(), and container/heap
// percolates it into the correct location.
// 使用最小堆的方式定义一个waitFor的优先级队列
type waitForPriorityQueue []*waitFor

func (pq waitForPriorityQueue) Len() int {
	return len(pq)
}
func (pq waitForPriorityQueue) Less(i, j int) bool {
	return pq[i].readyAt.Before(pq[j].readyAt)
}
func (pq waitForPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

// Push adds an item to the queue. Push should not be called directly; instead,
// use `heap.Push`.
func (pq *waitForPriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*waitFor)
	item.index = n
	*pq = append(*pq, item)
}

// Pop removes an item from the queue. Pop should not be called directly;
// instead, use `heap.Pop`.
func (pq *waitForPriorityQueue) Pop() interface{} {
	n := len(*pq)
	item := (*pq)[n-1]
	item.index = -1
	*pq = (*pq)[0:(n - 1)]
	return item
}

// Peek returns the item at the beginning of the queue, without removing the
// item or otherwise mutating the queue. It is safe to call directly.
// 获取队列第一个元素
func (pq waitForPriorityQueue) Peek() interface{} {
	return pq[0]
}

// ShutDown stops the queue. After the queue drains, the returned shutdown bool
// on Get() will be true. This method may be invoked more than once.
func (q *delayingType) ShutDown() {
	q.stopOnce.Do(func() {
		q.Interface.ShutDown()
		close(q.stopCh)
		q.heartbeat.Stop()
	})
}

// AddAfter adds the given item to the work queue after the given delay
// 在指定延时时间到达后，在workQueue中添加一个元素
func (q *delayingType) AddAfter(item interface{}, duration time.Duration) {
	// don't add if we're already shutting down
	if q.ShuttingDown() {
		return
	}

	q.metrics.retry()

	// immediately add things with no delay
	if duration <= 0 {
		q.Add(item) // 如果时间到了，就立刻添加
		return
	}

	select {
	case <-q.stopCh:
		// unblock if ShutDown() is called
	//	构造waitFor丢到waitingForAddCh中
	case q.waitingForAddCh <- &waitFor{data: item, readyAt: q.clock.Now().Add(duration)}:
	}
}

// maxWait keeps a max bound on the wait time. It's just insurance against weird things happening.
// Checking the queue every 10 seconds isn't expensive and we know that we'll never end up with an
// expired item sitting for more than 10 seconds.
const maxWait = 10 * time.Second

// waitingLoop runs until the workqueue is shutdown and keeps a check on the list of items to be added.
func (q *delayingType) waitingLoop() {
	defer utilruntime.HandleCrash()

	// Make a placeholder channel to use when there are no items in our list
	// 队列里没有元素时实现等待
	never := make(<-chan time.Time)

	// Make a timer that expires when the item at the head of the waiting queue is ready
	var nextReadyAtTimer clock.Timer

	// 构建一个优先级队列
	waitingForQueue := &waitForPriorityQueue{}
	heap.Init(waitingForQueue)

	waitingEntryByData := map[t]*waitFor{} // 用来处理重复添加逻辑

	for {
		if q.Interface.ShuttingDown() {
			return
		}

		now := q.clock.Now()

		// Add ready entries
		for waitingForQueue.Len() > 0 {
			entry := waitingForQueue.Peek().(*waitFor) // 获取第一个元素
			if entry.readyAt.After(now) {              // 时间还没到，先不处理
				break
			}

			entry = heap.Pop(waitingForQueue).(*waitFor) // 时间到了，pop出第一个元素。waitingForQueue.Pop()是最后一个元素，heap.Pop()是第一个元素
			q.Add(entry.data)                            // 将数据添加到延时队列里
			delete(waitingEntryByData, entry.data)       // 在map中删除已经加到延时队列的元素
		}

		// Set up a wait for the first item's readyAt (if one exists)
		// 如果队列中有元素，就用第一个元素的等待时间初始化计时器，如果为空则一直等待
		nextReadyAt := never
		if waitingForQueue.Len() > 0 {
			if nextReadyAtTimer != nil {
				nextReadyAtTimer.Stop()
			}
			entry := waitingForQueue.Peek().(*waitFor)
			nextReadyAtTimer = q.clock.NewTimer(entry.readyAt.Sub(now))
			nextReadyAt = nextReadyAtTimer.C()
		}

		select {
		case <-q.stopCh:
			return

		case <-q.heartbeat.C(): // 心跳时间10s，到了就继续下一轮循环
			// continue the loop, which will add ready items

		case <-nextReadyAt: // 第一个元素的等待时间到了，继续下一轮循环
			// continue the loop, which will add ready items

		//	waitingForAddCh收到新的元素
		case waitEntry := <-q.waitingForAddCh:
			// 如果时间没到，就加到优先级队列里，如果时间到了，就直接加到延时队列里
			if waitEntry.readyAt.After(q.clock.Now()) {
				insert(waitingForQueue, waitingEntryByData, waitEntry)
			} else {
				q.Add(waitEntry.data)
			}

			// 下面的逻辑就是将waitingForAddCh中的数据处理完
			drained := false
			for !drained {
				select {
				case waitEntry := <-q.waitingForAddCh:
					if waitEntry.readyAt.After(q.clock.Now()) {
						insert(waitingForQueue, waitingEntryByData, waitEntry)
					} else {
						q.Add(waitEntry.data)
					}
				default:
					drained = true
				}
			}
		}
	}
}

// insert adds the entry to the priority queue, or updates the readyAt if it already exists in the queue
func insert(q *waitForPriorityQueue, knownEntries map[t]*waitFor, entry *waitFor) {
	// if the entry already exists, update the time only if it would cause the item to be queued sooner
	// 如果entry已经存在，新的entry的就绪时间更短，就更新时间
	existing, exists := knownEntries[entry.data]
	if exists {
		if existing.readyAt.After(entry.readyAt) {
			existing.readyAt = entry.readyAt // 如果存在就只更新时间
			heap.Fix(q, existing.index)
		}

		return
	}

	// 如果不存在就丢到q里，同时在map中记录一下，用于查重
	heap.Push(q, entry)
	knownEntries[entry.data] = entry
}
