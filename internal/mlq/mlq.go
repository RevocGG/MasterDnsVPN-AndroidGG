// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package mlq

import (
	"container/list"
	"math/bits"
	"sync"
	"sync/atomic"
)

type PriorityQueue[T any] struct {
	items *list.List
}

type censusEntry[T any] struct {
	priority int
	elem     *list.Element
}

type queueEntry[T any] struct {
	key  uint64
	item T
}

// MultiLevelQueue is a thread-safe, multi-priority queue.
// Lower priority numbers are higher priority.
type MultiLevelQueue[T any] struct {
	mu sync.RWMutex

	queues   [6]PriorityQueue[T]
	bitmask  uint16
	fastSize atomic.Int32

	// O(1) existence and direct removal by key.
	census map[uint64]censusEntry[T]
}

func New[T any](initialCapacity int) *MultiLevelQueue[T] {
	m := &MultiLevelQueue[T]{
		census: make(map[uint64]censusEntry[T], initialCapacity),
	}
	for i := range m.queues {
		m.queues[i].items = list.New()
	}
	return m
}

func (m *MultiLevelQueue[T]) Push(priority int, key uint64, item T) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.census[key]; exists {
		return false
	}

	if priority < 0 || priority >= len(m.queues) {
		priority = 3
	}

	elem := m.queues[priority].items.PushBack(queueEntry[T]{
		key:  key,
		item: item,
	})
	m.census[key] = censusEntry[T]{
		priority: priority,
		elem:     elem,
	}
	m.bitmask |= (1 << uint(priority))
	m.fastSize.Add(1)
	return true
}

func (m *MultiLevelQueue[T]) Pop() (T, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.popLocked()
}

func (m *MultiLevelQueue[T]) Peek() (T, int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var zero T
	if m.bitmask == 0 {
		return zero, 0, false
	}

	tempMask := m.bitmask
	for tempMask != 0 {
		priority := bits.TrailingZeros16(tempMask)
		q := &m.queues[priority]
		front := q.items.Front()
		if front != nil {
			entry := front.Value.(queueEntry[T])
			return entry.item, priority, true
		}
		tempMask &= ^(1 << uint(priority))
	}

	return zero, 0, false
}

func (m *MultiLevelQueue[T]) popLocked() (T, int, bool) {
	var zero T
	for m.bitmask != 0 {
		priority := bits.TrailingZeros16(m.bitmask)
		q := &m.queues[priority]
		front := q.items.Front()
		if front == nil {
			m.bitmask &= ^(1 << uint(priority))
			continue
		}

		entry := front.Value.(queueEntry[T])
		q.items.Remove(front)
		delete(m.census, entry.key)
		m.fastSize.Add(-1)
		if q.items.Len() == 0 {
			m.bitmask &= ^(1 << uint(priority))
		}
		return entry.item, priority, true
	}

	return zero, 0, false
}

func (m *MultiLevelQueue[T]) Get(key uint64) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var zero T
	entry, exists := m.census[key]
	if !exists || entry.elem == nil {
		return zero, false
	}
	value, ok := entry.elem.Value.(queueEntry[T])
	if !ok {
		return zero, false
	}
	return value.item, true
}

func (m *MultiLevelQueue[T]) RemoveByKey(key uint64) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var zero T
	entry, exists := m.census[key]
	if !exists || entry.elem == nil || entry.priority < 0 || entry.priority >= len(m.queues) {
		return zero, false
	}

	value, ok := entry.elem.Value.(queueEntry[T])
	if !ok {
		return zero, false
	}

	q := &m.queues[entry.priority]
	q.items.Remove(entry.elem)
	delete(m.census, key)
	m.fastSize.Add(-1)
	if q.items.Len() == 0 {
		m.bitmask &= ^(1 << uint(entry.priority))
	}
	return value.item, true
}

func (m *MultiLevelQueue[T]) Count(priority int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if priority < 0 || priority >= len(m.queues) {
		return 0
	}
	return m.queues[priority].items.Len()
}

func (m *MultiLevelQueue[T]) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.census)
}

func (m *MultiLevelQueue[T]) FastSize() int {
	if m == nil {
		return 0
	}
	return int(m.fastSize.Load())
}

func (m *MultiLevelQueue[T]) Clear(callback func(T)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.queues {
		if callback != nil {
			for elem := m.queues[i].items.Front(); elem != nil; elem = elem.Next() {
				entry := elem.Value.(queueEntry[T])
				callback(entry.item)
			}
		}
		m.queues[i].items.Init()
	}
	clear(m.census)
	m.bitmask = 0
	m.fastSize.Store(0)
}

func (m *MultiLevelQueue[T]) HighestPriority() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.bitmask == 0 {
		return -1
	}
	return bits.TrailingZeros16(m.bitmask)
}

func (m *MultiLevelQueue[T]) PopIf(priority int, predicate func(T) bool, keyExtractor func(T) uint64) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var zero T
	if m.bitmask == 0 || priority < 0 || priority >= len(m.queues) {
		return zero, false
	}
	if (m.bitmask & (1 << uint(priority))) == 0 {
		return zero, false
	}

	q := &m.queues[priority]
	front := q.items.Front()
	if front == nil {
		m.bitmask &= ^(1 << uint(priority))
		return zero, false
	}

	entry := front.Value.(queueEntry[T])
	if predicate != nil && !predicate(entry.item) {
		return zero, false
	}

	q.items.Remove(front)
	delete(m.census, entry.key)
	m.fastSize.Add(-1)
	if q.items.Len() == 0 {
		m.bitmask &= ^(1 << uint(priority))
	}
	return entry.item, true
}

func (m *MultiLevelQueue[T]) PopAnyIf(maxPriority int, predicate func(T) bool, keyExtractor func(T) uint64) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var zero T
	if m.bitmask == 0 {
		return zero, false
	}

	tempMask := m.bitmask
	for tempMask != 0 {
		priority := bits.TrailingZeros16(tempMask)
		if priority > maxPriority {
			break
		}

		q := &m.queues[priority]
		for elem := q.items.Front(); elem != nil; elem = elem.Next() {
			entry := elem.Value.(queueEntry[T])
			if predicate != nil && !predicate(entry.item) {
				continue
			}

			q.items.Remove(elem)
			delete(m.census, entry.key)
			m.fastSize.Add(-1)
			if q.items.Len() == 0 {
				m.bitmask &= ^(1 << uint(priority))
			}
			return entry.item, true
		}

		tempMask &= ^(1 << uint(priority))
	}

	return zero, false
}
