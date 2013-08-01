// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/watcher"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
)

type notifyWorkerSuite struct {
	coretesting.LoggingSuite
	worker worker.NotifyWorker
	actor  *notifyHandler
}

var _ = gc.Suite(&notifyWorkerSuite{})

func (s *notifyWorkerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.actor = &notifyHandler{
		actions:     nil,
		handled:     make(chan struct{}, 1),
		description: "test notify handler",
		watcher: &testNotifyWatcher{
			changes: make(chan struct{}),
		},
	}
	s.worker = worker.NewNotifyWorker(s.actor)
}

func (s *notifyWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.LoggingSuite.TearDownTest(c)
}

type notifyHandler struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled       chan struct{}
	setupError    error
	teardownError error
	handlerError  error
	watcher       *testNotifyWatcher
	description   string
}

var _ worker.NotifyWatchHandler = (*notifyHandler)(nil)

func (nh *notifyHandler) SetUp() (api.NotifyWatcher, error) {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "setup")
	if nh.watcher == nil {
		return nil, nh.setupError
	}
	return nh.watcher, nh.setupError
}

func (nh *notifyHandler) TearDown() error {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "teardown")
	if nh.handled != nil {
		close(nh.handled)
	}
	return nh.teardownError
}

func (nh *notifyHandler) Handle() error {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "handler")
	if nh.handled != nil {
		// Unlock while we are waiting for the send
		nh.mu.Unlock()
		nh.handled <- struct{}{}
		nh.mu.Lock()
	}
	return nh.handlerError
}

func (nh *notifyHandler) String() string {
	return nh.description
}

func (nh *notifyHandler) CheckActions(c *gc.C, actions ...string) {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	c.Check(nh.actions, gc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *notifyWorkerSuite) stopWorker(c *gc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- s.worker.Stop()
	}()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Check(err, gc.IsNil)
	s.actor = nil
	s.worker = nil
}

type testNotifyWatcher struct {
	mu        sync.Mutex
	changes   chan struct{}
	stopped   bool
	stopError error
}

var _ api.NotifyWatcher = (*testNotifyWatcher)(nil)

func (tnw *testNotifyWatcher) Changes() <-chan struct{} {
	return tnw.changes
}

func (tnw *testNotifyWatcher) Err() error {
	return tnw.stopError
}

func (tnw *testNotifyWatcher) Stop() error {
	tnw.mu.Lock()
	defer tnw.mu.Unlock()
	if !tnw.stopped {
		close(tnw.changes)
	}
	tnw.stopped = true
	return tnw.stopError
}

func (tnw *testNotifyWatcher) SetStopError(err error) {
	tnw.mu.Lock()
	tnw.stopError = err
	tnw.mu.Unlock()
}

func (tnw *testNotifyWatcher) TriggerChange(c *gc.C) {
	select {
	case tnw.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Errorf("Timeout changes triggering change after %s", coretesting.LongWait)
	}
}

func waitForTimeout(c *gc.C, ch <-chan error, timeout time.Duration) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		c.Errorf("failed to receive value after %s", timeout)
	}
	return nil
}

func waitShort(c *gc.C, w worker.CommonWorker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	return waitForTimeout(c, done, coretesting.ShortWait)
}

func waitForHandledNotify(c *gc.C, handled chan struct{}) {
	select {
	case <-handled:
		return
	case <-time.After(coretesting.LongWait):
		c.Errorf("handled failed to signal after %s", coretesting.LongWait)
	}
}

func (s *notifyWorkerSuite) TestKill(c *gc.C) {
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestStop(c *gc.C) {
	err := s.worker.Stop()
	c.Assert(err, gc.IsNil)
	// After stop, Wait should return right away
	err = waitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestWait(c *gc.C) {
	done := make(chan error)
	go func() {
		done <- s.worker.Wait()
	}()
	// Wait should not return until we've killed the worker
	select {
	case err := <-done:
		c.Errorf("Wait() didn't wait until we stopped it: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	s.worker.Kill()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestStringForwardsHandlerString(c *gc.C) {
	c.Check(fmt.Sprint(s.worker), gc.Equals, "test notify handler")
}

func (s *notifyWorkerSuite) TestCallSetUpAndTearDown(c *gc.C) {
	// After calling NewNotifyWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(s.worker.Stop(), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *notifyWorkerSuite) TestSetUpFailureStopsWithTearDown(c *gc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor := &notifyHandler{
		actions:    nil,
		handled:    make(chan struct{}, 1),
		setupError: fmt.Errorf("my special error"),
		watcher: &testNotifyWatcher{
			changes: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.actor.watcher.SetStopError(fmt.Errorf("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *notifyWorkerSuite) TestCleanRunNoticesTearDownError(c *gc.C) {
	s.actor.teardownError = fmt.Errorf("failed to tear down watcher")
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "failed to tear down watcher")
	s.worker = nil
}

func (s *notifyWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor := &notifyHandler{
		actions:      nil,
		handled:      make(chan struct{}, 1),
		handlerError: fmt.Errorf("my handling error"),
		watcher: &testNotifyWatcher{
			changes: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, actor.handled)
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestNoticesStoppedWatcher(c *gc.C) {
	// The default closedHandler doesn't panic if you have a genuine error
	// (because it assumes you want to propagate a real error and then
	// restart
	s.actor.watcher.SetStopError(fmt.Errorf("Stopped Watcher"))
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	c.Check(err, gc.ErrorMatches, "Stopped Watcher")
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func noopHandler(watcher.Errer) error {
	return nil
}

type CannedErrer struct {
	err error
}

func (c CannedErrer) Err() error {
	return c.err
}

type closerHandler interface {
	SetClosedHandler(func(watcher.Errer) error) func(watcher.Errer) error
}

func (s *notifyWorkerSuite) TestDefaultClosedHandler(c *gc.C) {
	h, ok := s.worker.(closerHandler)
	c.Assert(ok, jc.IsTrue)
	old := h.SetClosedHandler(noopHandler)
	noErr := CannedErrer{nil}
	stillAlive := CannedErrer{tomb.ErrStillAlive}
	customErr := CannedErrer{fmt.Errorf("my special error")}

	// The default handler should be watcher.MustErr which panics if the
	// Errer doesn't actually have an error
	c.Assert(func() { old(noErr) }, gc.PanicMatches, "watcher was stopped cleanly")
	c.Assert(func() { old(stillAlive) }, gc.PanicMatches, "watcher is still running")
	c.Assert(old(customErr), gc.Equals, customErr.Err())
}

func (s *notifyWorkerSuite) TestErrorsOnStillAliveButClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	s.worker.(closerHandler).SetClosedHandler(triggeredHandler)
	s.actor.watcher.SetStopError(tomb.ErrStillAlive)
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	c.Check(foundErr, gc.Equals, tomb.ErrStillAlive)
	// ErrStillAlive is trapped by the Stop logic and gets turned into a
	// 'nil' when stopping. However TestDefaultClosedHandler can assert
	// that it would have triggered a panic.
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func (s *notifyWorkerSuite) TestErrorsOnClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	s.worker.(closerHandler).SetClosedHandler(triggeredHandler)
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	// If the foundErr is nil, we would have panic-ed (see TestDefaultClosedHandler)
	c.Check(foundErr, gc.IsNil)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
}
