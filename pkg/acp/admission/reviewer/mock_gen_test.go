// Code generated by mocktail; DO NOT EDIT.

package reviewer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
)

// ingressClassesMock mock of IngressClasses.
type ingressClassesMock struct{ mock.Mock }

// newIngressClassesMock creates a new ingressClassesMock.
func newIngressClassesMock(tb testing.TB) *ingressClassesMock {
	tb.Helper()

	m := &ingressClassesMock{}
	m.Mock.Test(tb)

	tb.Cleanup(func() { m.AssertExpectations(tb) })

	return m
}

func (_m *ingressClassesMock) GetController(name string) (string, error) {
	_ret := _m.Called(name)

	if _rf, ok := _ret.Get(0).(func(string) (string, error)); ok {
		return _rf(name)
	}

	_ra0 := _ret.String(0)
	_rb1 := _ret.Error(1)

	return _ra0, _rb1
}

func (_m *ingressClassesMock) OnGetController(name string) *ingressClassesGetControllerCall {
	return &ingressClassesGetControllerCall{Call: _m.Mock.On("GetController", name), Parent: _m}
}

func (_m *ingressClassesMock) OnGetControllerRaw(name interface{}) *ingressClassesGetControllerCall {
	return &ingressClassesGetControllerCall{Call: _m.Mock.On("GetController", name), Parent: _m}
}

type ingressClassesGetControllerCall struct {
	*mock.Call
	Parent *ingressClassesMock
}

func (_c *ingressClassesGetControllerCall) Panic(msg string) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *ingressClassesGetControllerCall) Once() *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *ingressClassesGetControllerCall) Twice() *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *ingressClassesGetControllerCall) Times(i int) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *ingressClassesGetControllerCall) WaitUntil(w <-chan time.Time) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *ingressClassesGetControllerCall) After(d time.Duration) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *ingressClassesGetControllerCall) Run(fn func(args mock.Arguments)) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *ingressClassesGetControllerCall) Maybe() *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Maybe()
	return _c
}

func (_c *ingressClassesGetControllerCall) TypedReturns(a string, b error) *ingressClassesGetControllerCall {
	_c.Call = _c.Return(a, b)
	return _c
}

func (_c *ingressClassesGetControllerCall) ReturnsFn(fn func(string) (string, error)) *ingressClassesGetControllerCall {
	_c.Call = _c.Return(fn)
	return _c
}

func (_c *ingressClassesGetControllerCall) TypedRun(fn func(string)) *ingressClassesGetControllerCall {
	_c.Call = _c.Call.Run(func(args mock.Arguments) {
		_name := args.String(0)
		fn(_name)
	})
	return _c
}

func (_c *ingressClassesGetControllerCall) OnGetController(name string) *ingressClassesGetControllerCall {
	return _c.Parent.OnGetController(name)
}

func (_c *ingressClassesGetControllerCall) OnGetDefaultController() *ingressClassesGetDefaultControllerCall {
	return _c.Parent.OnGetDefaultController()
}

func (_c *ingressClassesGetControllerCall) OnGetControllerRaw(name interface{}) *ingressClassesGetControllerCall {
	return _c.Parent.OnGetControllerRaw(name)
}

func (_c *ingressClassesGetControllerCall) OnGetDefaultControllerRaw() *ingressClassesGetDefaultControllerCall {
	return _c.Parent.OnGetDefaultControllerRaw()
}

func (_m *ingressClassesMock) GetDefaultController() (string, error) {
	_ret := _m.Called()

	if _rf, ok := _ret.Get(0).(func() (string, error)); ok {
		return _rf()
	}

	_ra0 := _ret.String(0)
	_rb1 := _ret.Error(1)

	return _ra0, _rb1
}

func (_m *ingressClassesMock) OnGetDefaultController() *ingressClassesGetDefaultControllerCall {
	return &ingressClassesGetDefaultControllerCall{Call: _m.Mock.On("GetDefaultController"), Parent: _m}
}

func (_m *ingressClassesMock) OnGetDefaultControllerRaw() *ingressClassesGetDefaultControllerCall {
	return &ingressClassesGetDefaultControllerCall{Call: _m.Mock.On("GetDefaultController"), Parent: _m}
}

type ingressClassesGetDefaultControllerCall struct {
	*mock.Call
	Parent *ingressClassesMock
}

func (_c *ingressClassesGetDefaultControllerCall) Panic(msg string) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) Once() *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) Twice() *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) Times(i int) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) WaitUntil(w <-chan time.Time) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) After(d time.Duration) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) Run(fn func(args mock.Arguments)) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) Maybe() *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Maybe()
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) TypedReturns(a string, b error) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Return(a, b)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) ReturnsFn(fn func() (string, error)) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Return(fn)
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) TypedRun(fn func()) *ingressClassesGetDefaultControllerCall {
	_c.Call = _c.Call.Run(func(args mock.Arguments) {
		fn()
	})
	return _c
}

func (_c *ingressClassesGetDefaultControllerCall) OnGetController(name string) *ingressClassesGetControllerCall {
	return _c.Parent.OnGetController(name)
}

func (_c *ingressClassesGetDefaultControllerCall) OnGetDefaultController() *ingressClassesGetDefaultControllerCall {
	return _c.Parent.OnGetDefaultController()
}

func (_c *ingressClassesGetDefaultControllerCall) OnGetControllerRaw(name interface{}) *ingressClassesGetControllerCall {
	return _c.Parent.OnGetControllerRaw(name)
}

func (_c *ingressClassesGetDefaultControllerCall) OnGetDefaultControllerRaw() *ingressClassesGetDefaultControllerCall {
	return _c.Parent.OnGetDefaultControllerRaw()
}

// policyGetterMock mock of PolicyGetter.
type policyGetterMock struct{ mock.Mock }

// newPolicyGetterMock creates a new policyGetterMock.
func newPolicyGetterMock(tb testing.TB) *policyGetterMock {
	tb.Helper()

	m := &policyGetterMock{}
	m.Mock.Test(tb)

	tb.Cleanup(func() { m.AssertExpectations(tb) })

	return m
}

func (_m *policyGetterMock) GetConfig(canonicalName string) (*acp.Config, error) {
	_ret := _m.Called(canonicalName)

	if _rf, ok := _ret.Get(0).(func(string) (*acp.Config, error)); ok {
		return _rf(canonicalName)
	}

	_ra0, _ := _ret.Get(0).(*acp.Config)
	_rb1 := _ret.Error(1)

	return _ra0, _rb1
}

func (_m *policyGetterMock) OnGetConfig(canonicalName string) *policyGetterGetConfigCall {
	return &policyGetterGetConfigCall{Call: _m.Mock.On("GetConfig", canonicalName), Parent: _m}
}

func (_m *policyGetterMock) OnGetConfigRaw(canonicalName interface{}) *policyGetterGetConfigCall {
	return &policyGetterGetConfigCall{Call: _m.Mock.On("GetConfig", canonicalName), Parent: _m}
}

type policyGetterGetConfigCall struct {
	*mock.Call
	Parent *policyGetterMock
}

func (_c *policyGetterGetConfigCall) Panic(msg string) *policyGetterGetConfigCall {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *policyGetterGetConfigCall) Once() *policyGetterGetConfigCall {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *policyGetterGetConfigCall) Twice() *policyGetterGetConfigCall {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *policyGetterGetConfigCall) Times(i int) *policyGetterGetConfigCall {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *policyGetterGetConfigCall) WaitUntil(w <-chan time.Time) *policyGetterGetConfigCall {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *policyGetterGetConfigCall) After(d time.Duration) *policyGetterGetConfigCall {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *policyGetterGetConfigCall) Run(fn func(args mock.Arguments)) *policyGetterGetConfigCall {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *policyGetterGetConfigCall) Maybe() *policyGetterGetConfigCall {
	_c.Call = _c.Call.Maybe()
	return _c
}

func (_c *policyGetterGetConfigCall) TypedReturns(a *acp.Config, b error) *policyGetterGetConfigCall {
	_c.Call = _c.Return(a, b)
	return _c
}

func (_c *policyGetterGetConfigCall) ReturnsFn(fn func(string) (*acp.Config, error)) *policyGetterGetConfigCall {
	_c.Call = _c.Return(fn)
	return _c
}

func (_c *policyGetterGetConfigCall) TypedRun(fn func(string)) *policyGetterGetConfigCall {
	_c.Call = _c.Call.Run(func(args mock.Arguments) {
		_canonicalName := args.String(0)
		fn(_canonicalName)
	})
	return _c
}

func (_c *policyGetterGetConfigCall) OnGetConfig(canonicalName string) *policyGetterGetConfigCall {
	return _c.Parent.OnGetConfig(canonicalName)
}

func (_c *policyGetterGetConfigCall) OnGetConfigRaw(canonicalName interface{}) *policyGetterGetConfigCall {
	return _c.Parent.OnGetConfigRaw(canonicalName)
}
