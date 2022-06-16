// Code generated by mocktail; DO NOT EDIT.

package acp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

// clientMock mock of Client.
type clientMock struct{ mock.Mock }

// newClientMock creates a new clientMock.
func newClientMock(tb testing.TB) *clientMock {
	tb.Helper()

	m := &clientMock{}
	m.Mock.Test(tb)

	tb.Cleanup(func() { m.AssertExpectations(tb) })

	return m
}

func (_m *clientMock) GetACPs(_ context.Context) ([]ACP, error) {
	_ret := _m.Called()

	_ra0, _ := _ret.Get(0).([]ACP)
	_rb1 := _ret.Error(1)

	return _ra0, _rb1
}

func (_m *clientMock) OnGetACPs() *clientGetACPsCall {
	return &clientGetACPsCall{Call: _m.Mock.On("GetACPs"), Parent: _m}
}

func (_m *clientMock) OnGetACPsRaw() *clientGetACPsCall {
	return &clientGetACPsCall{Call: _m.Mock.On("GetACPs"), Parent: _m}
}

type clientGetACPsCall struct {
	*mock.Call
	Parent *clientMock
}

func (_c *clientGetACPsCall) Panic(msg string) *clientGetACPsCall {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *clientGetACPsCall) Once() *clientGetACPsCall {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *clientGetACPsCall) Twice() *clientGetACPsCall {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *clientGetACPsCall) Times(i int) *clientGetACPsCall {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *clientGetACPsCall) WaitUntil(w <-chan time.Time) *clientGetACPsCall {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *clientGetACPsCall) After(d time.Duration) *clientGetACPsCall {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *clientGetACPsCall) Run(fn func(args mock.Arguments)) *clientGetACPsCall {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *clientGetACPsCall) Maybe() *clientGetACPsCall {
	_c.Call = _c.Call.Maybe()
	return _c
}

func (_c *clientGetACPsCall) TypedReturns(a []ACP, b error) *clientGetACPsCall {
	_c.Call = _c.Return(a, b)
	return _c
}

func (_c *clientGetACPsCall) ReturnsFn(fn func() ([]ACP, error)) *clientGetACPsCall {
	_c.Call = _c.Return(fn)
	return _c
}

func (_c *clientGetACPsCall) TypedRun(fn func()) *clientGetACPsCall {
	_c.Call = _c.Call.Run(func(args mock.Arguments) {
		fn()
	})
	return _c
}

func (_c *clientGetACPsCall) OnGetACPs() *clientGetACPsCall {
	return _c.Parent.OnGetACPs()
}

func (_c *clientGetACPsCall) OnGetACPsRaw() *clientGetACPsCall {
	return _c.Parent.OnGetACPsRaw()
}