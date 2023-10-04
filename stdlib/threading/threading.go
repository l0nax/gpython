package threading

import (
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-python/gpython/py"
)

const debugging = false

var PyThreadType = py.NewType("Thread", "")

//go:embed threading.py
var threadModSrc string

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

// Thread represents the Thread class.
type Thread struct {
	target *py.Function
	args   py.Tuple

	state atomic.Int32
	wg    sync.WaitGroup
}

func (t *Thread) Type() *py.Type {
	return PyThreadType
}

// start executes the function asynchronously.
func (t *Thread) start() error {
	if t.target == nil {
		return nil
	}

	if !t.state.CompareAndSwap(stateInitialized, stateRunning) {
		return py.ExceptionNewf(py.RuntimeError, "start() can only be called once!")
	}

	t.wg.Add(1)
	go func() {
		py.Call(t.target, t.args, nil)

		t.state.Store(stateFinished)
		t.wg.Done()
	}()

	return nil
}

// ThreadNew is the Thread_new function.
func ThreadNew(_ py.Object, _ py.Tuple) (py.Object, error) {
	return &Thread{}, nil
}

// ThreadSetTarget is a mapper of Thread.set_target(...).
func ThreadSetTarget(self py.Object, args py.Tuple) (py.Object, error) {
	th := self.(*Thread)

	fn, ok := args[0].(*py.Function)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected function, got %T", args[0])
	}

	th.target = fn

	return py.None, nil
}

// ThreadIsAlive is the mapper function for Thread.is_alive().
func ThreadIsAlive(self py.Object, _ py.Tuple) (py.Object, error) {
	state := self.(*Thread).state.Load()
	return py.NewBool(state != stateFinished), nil
}

// ThreadJoin is the mapper function of Thread.join(...).
func ThreadJoin(self py.Object, args py.Tuple) (py.Object, error) {
	th := self.(*Thread)
	if th.state.Load() == stateInitialized {
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
