// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package etcd

import (
	"fmt"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// task represents a single task to run. These are useful for pending work that
// we want to schedule, but that shouldn't permanently error the system on
// error. In particular idempotent tasks that are safe are ideal for this queue.
// The tasks can be added with queueTask.
type task struct {
	name   string       // name of task
	fn     func() error // task to run
	retry  int          // number of times to retry on error, -1 for infinite
	block  bool         // should we block the queue until this succeeds?
	report bool         // should we report the error on permanent failure?
}

// String prints a string representation of the struct.
func (obj *task) String() string {
	return fmt.Sprintf("task(%s)", obj.name)
}

// queueTask adds a task to the task worker queue. If you want to specify any
// properties that differ from the defaults, use queueRawTask instead.
func (obj *EmbdEtcd) queueTask(fn func() error) error {
	obj.taskQueueLock.Lock()
	obj.taskQueueLock.Unlock()
	t := &task{
		fn: fn,
	}
	return obj.queueRawTask(t)
}

// queueRawTask adds a task of any format to the queue. You should not name your
// task a string which could match a positive integer. Those names are used when
// an unnamed task is specified and the system needs to generate a name.
func (obj *EmbdEtcd) queueRawTask(t *task) error {
	if obj.Debug {
		obj.Logf("queueRawTask()")
		defer obj.Logf("queueRawTask(): done!")
	}

	if t == nil {
		return fmt.Errorf("nil task")
	}

	obj.taskQueueLock.Lock()
	defer obj.taskQueueLock.Unlock()
	if obj.taskQueue == nil { // killed signal
		return fmt.Errorf("task queue killed")
	}
	if t.name == "" {
		obj.taskQueueID++ // increment
		t.name = fmt.Sprintf("%d", obj.taskQueueID)
	}

	obj.taskQueue = append(obj.taskQueue, t)
	if !obj.taskQueueRunning {
		obj.taskQueueRunning = true
		obj.taskQueueWg.Add(1)
		go obj.runTaskQueue()
	}
	return nil
}

// killTaskQueue empties the task queue, causing it to shutdown.
func (obj *EmbdEtcd) killTaskQueue() int {
	obj.taskQueueLock.Lock()
	count := len(obj.taskQueue)
	obj.taskQueue = nil // clear queue
	obj.taskQueueLock.Unlock()

	obj.taskQueueWg.Wait()    // wait for queue to exit
	obj.taskQueue = []*task{} // reset
	return count              // number of tasks deleted
}

// runTaskQueue processes the task queue. This is started automatically by
// queueTask if needed. It will shut itself down when the queue is empty.
func (obj *EmbdEtcd) runTaskQueue() {
	defer obj.taskQueueWg.Done() // added in queueTask
	for {
		obj.taskQueueLock.Lock()
		if obj.taskQueue == nil || len(obj.taskQueue) == 0 {
			defer obj.taskQueueLock.Unlock()
			obj.taskQueueRunning = false
			return
		}
		var t *task
		t, obj.taskQueue = obj.taskQueue[0], obj.taskQueue[1:]
		obj.taskQueueLock.Unlock()

		if !t.block {
			if obj.Debug {
				obj.Logf("%s: run...", t)
			}
			err := t.fn()
			if obj.Debug {
				obj.Logf("%s: done: %v", t, err)
			}
			if err != nil {
				if t.retry == 0 {
					if t.report {
						// send a permanent error
						// XXX: guard errChan for early close... hmmm
						select {
						case obj.errChan <- errwrap.Wrapf(err, "task error"):
						}
					}
					continue
				}
				if t.retry > 0 { // don't decrement from -1
					t.retry--
				}
				obj.taskQueueLock.Lock()
				if obj.taskQueue != nil { // killed signal
					obj.taskQueue = append(obj.taskQueue, t)
				}
				obj.taskQueueLock.Unlock()
			}
			continue
		}

		// block
		for {
			if obj.Debug {
				obj.Logf("%s: run...", t)
			}
			err := t.fn()
			if obj.Debug {
				obj.Logf("%s: done: %v", t, err)
			}
			if err != nil {
				if t.retry == 0 {
					break
				}
				if t.retry > 0 { // don't decrement from -1
					t.retry--
				}
			}
		}
	}
}
