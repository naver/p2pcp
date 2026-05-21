// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"math/rand"
	"sync"
	"time"
)

type ChunkQueue struct {
	queue []int
	rand  *rand.Rand
	lock  sync.RWMutex
}

func NewChunkQueue() *ChunkQueue {
	return &ChunkQueue{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (queue *ChunkQueue) PushBack(index int) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queue.queue = append(queue.queue, index)
}

func (queue *ChunkQueue) PushShuffledList(indices []int) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	queue.queue = append(queue.queue, indices...)
}

func (queue *ChunkQueue) PushShuffle(index int) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	queue.queue = append(queue.queue, index)

	lenQueue := len(queue.queue)
	indexInsert := queue.rand.Int() % (lenQueue)
	(queue.queue)[indexInsert], (queue.queue)[lenQueue-1] = (queue.queue)[lenQueue-1], (queue.queue)[indexInsert]
}

func (queue *ChunkQueue) Pop() (int, bool) {
	queue.lock.Lock()
	defer queue.lock.Unlock()

	if len(queue.queue) != 0 {
		index := (queue.queue)[0]
		queue.queue = (queue.queue)[1:]
		return index, true
	}

	return 0, false
}
