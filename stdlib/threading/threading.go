package threading

import (
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-python/gpython/py"
)

const (
	debugging        = true
	annotThreadState = "__gpy_thread_state"
)

var (
	PyThreadType  = py.NewType("Thread", "")
	PyThreadState = py.NewType("ThreadState", "internal state representation of Thread")
)

var (
	//go:embed threading.py
	threadModSrc string

	// tidCounter is the global thread ID counter.
	tidCounter atomic.Uint32

	threadMap = make(map[uint32]*Thread)
	threadLo  sync.Mutex
)

const (
	// stateInitialized is the base state.
	stateInitialized int32 = iota
	// stateRunning describes the case when the thread
	// is currently running.
	stateRunning
	// stateFinished describes the case when the function
	// was already executed.
	stateFinished
)

func init() {
	PyThreadType.Dict["set_target"] = py.MustNewMethod("Thread.set_target", ThreadSetTarget, 0, "")
	PyThreadType.Dict["set_args"] = py.MustNewMethod("Thread.set_args", ThreadSetArgs, 0, "")
	PyThreadType.Dict["join"] = py.MustNewMethod("Thread.join", ThreadJoin, 0, "")
	PyThreadType.Dict["is_alive"] = py.MustNewMethod("Thread.is_alive", ThreadIsAlive, 0, "")
	PyThreadType.Dict["start"] = py.MustNewMethod("Thread.start", ThreadStart, 0, "")

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "threading_go",
			Doc:  "",
		},
		Methods: []*py.Method{
			py.MustNewMethod("Thread_new", ThreadNew, 0, ""),
			py.MustNewMethod("Thread_current_thread", ThreadCurrentThread, 0, ""),
		},
	})

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "threading",
			Doc:  "",
		},
		CodeSrc: threadModSrc,
	})
}

// threadState is the thread state.
type threadState struct {
	lo sync.RWMutex

	// tid is the thread ID.
	// The ID starts from 1.
	// A 0 value indicates that the code is beeing executed
	// in the main (context) routine.
	tid uint32

	// state holds the thread state.
	// Please beware that this value is only meaningful
	// if tid > 0.
	state atomic.Int32
}

func (t *threadState) Type() *py.Type {
	return PyThreadState
}

// Thread represents the Thread class.
type Thread struct {
	target *py.Function
	args   py.Tuple

	state *threadState
	wg    sync.WaitGroup
}

// getState returns the thread state.
// The state is not to be confused with [threadState].
func (t *Thread) getState() int32 {
	return t.state.state.Load()
}

func (t *Thread) Type() *py.Type {
	return PyThreadType
}

// start executes the function asynchronously.
func (t *Thread) start() error {
	if t.target == nil {
		return nil
	}

	if !t.state.state.CompareAndSwap(stateInitialized, stateRunning) {
		return py.ExceptionNewf(py.RuntimeError, "start() can only be called once!")
	}

	t.wg.Add(1)
	go func() {
		obj, err := py.Call(t.target, t.args, nil)
		debugf("Returned (%T, %T) => %+v || %+v\n", obj, err, obj, err)

		t.state.state.Store(stateFinished)
		t.wg.Done()
	}()

	return nil
}

// ThreadCurrentThread implements the threading.current_thread() functionality.
func ThreadCurrentThread(obj py.Object, _ py.Tuple) (py.Object, error) {
	gd, ok := obj.(py.IGetDict)
	if !ok {
		return nil, py.ExceptionNewf(
			py.RuntimeError,
			"expected that obj implements py.IGetDict: %T",
			obj,
		)
	}

	raw, err := gd.GetDict().M__getitem__(py.String(annotThreadState))
	if err != nil {
		// not found => we're in the main thread.
		// Return a dummy [Thread] object
		return &Thread{
			state: &threadState{},
		}, nil
	}

	ts, ok := raw.(*threadState)
	if !ok {
		return nil, py.ExceptionNewf(py.RuntimeError, "expected internal threadState, got %T", raw)
	}

	threadLo.Lock()
	th, ok := threadMap[ts.tid]
	threadLo.Unlock()

	if !ok {
		return nil, py.ExceptionNewf(
			py.RuntimeError,
			"unable to find thread mapping of thread ID %d",
			ts.tid,
		)
	}

	return th, nil
}

// ThreadNew is the Thread_new function.
func ThreadNew(_ py.Object, _ py.Tuple) (py.Object, error) {
	return &Thread{
		state: &threadState{
			tid: tidCounter.Add(1),
		},
	}, nil
}

// ThreadSetTarget is a mapper of Thread.set_target(...).
func ThreadSetTarget(self py.Object, args py.Tuple) (py.Object, error) {
	th := self.(*Thread)

	fn, ok := args[0].(*py.Function)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected function, got %T", args[0])
	}

	debugf("Setting %T: %+v\n", fn, fn)
	th.target = fn

	// we annotate the function with the relevant information
	fn.GetDict().M__setitem__(py.String(annotThreadState), th.state)

	return py.None, nil
}

// ThreadIsAlive is the mapper function for Thread.is_alive().
func ThreadIsAlive(self py.Object, _ py.Tuple) (py.Object, error) {
	state := self.(*Thread).getState()
	return py.NewBool(state != stateFinished), nil
}

// ThreadJoin is the mapper function of Thread.join(...).
func ThreadJoin(self py.Object, args py.Tuple) (py.Object, error) {
	th := self.(*Thread)
	if th.getState() == stateInitialized {
		return nil, py.ExceptionNewf(
			py.RuntimeError.Base,
			"join() called before thread has been started",
		)
	}

	// simple case: no timeout passed
	if len(args) == 0 || args[0] == py.None {
		th.wg.Wait()

		return py.None, nil
	}

	secs, ok := args[0].(py.Float)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected float, got %T", args[0])
	}

	dur := time.Duration(float64(secs) * float64(time.Second))
	ch := make(chan struct{})

	ti := time.NewTimer(dur)
	defer ti.Stop()

	go func() {
		th.wg.Wait()
		close(ch)
	}()

	select {
	case <-ti.C:
		// timeout
		// TODO: Store information to return on is_alive() call
		return py.None, nil
	case <-ch:
		// stopped!
	}

	return py.None, nil
}

// ThreadSetArgs is the mapper function of Thread.set_args(...).
func ThreadSetArgs(self py.Object, args py.Tuple) (py.Object, error) {
	th := self.(*Thread)
	th.args = args

	return py.None, nil
}

// ThreadStart is a mapper of Thread.start().
func ThreadStart(self py.Object, _ py.Tuple) (py.Object, error) {
	th := self.(*Thread)
	th.start()

	return py.None, nil
}

func debugf(msg string, args ...any) {
	if debugging {
		fmt.Printf(msg, args...)
	}
}

func getAnnotations(obj py.Object) (py.StringDict, error) {
	raw, err := py.GetAttrString(obj, "__annotations__")
	if err != nil {
		return nil, err
	}

	return raw.(py.StringDict), nil
}
